// Package server serves the REST API, the WebSocket, and the embedded page (docs/design/protocol.md).
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/ryabinski-labs/ccctl/internal/auth"
	"github.com/ryabinski-labs/ccctl/internal/config"
	"github.com/ryabinski-labs/ccctl/internal/repos"
	"github.com/ryabinski-labs/ccctl/internal/session"
)

// Frame type bytes.
const (
	FrameOutput byte = 0x01
	FrameReplay byte = 0x02
)

type Server struct {
	M     *session.Manager
	Guard *auth.Guard
	Home  string
	// ScanTimeout stops a repo scan (default 10 s).
	ScanTimeout time.Duration
	Host        func() string
	Static      fs.FS // may be nil
	Log         *slog.Logger
	// Version is the ccctl build, sent in hello so stale pages can offer a reload.
	Version string

	mu      sync.Mutex
	clients map[*client]struct{}
}

// Routes lists every registered pattern (SC-005-h).
var Routes = []struct{ Method, Path string }{
	{"GET", "/"}, {"GET", "/assets/x.js"}, {"GET", "/api/state"}, {"GET", "/api/repos"}, {"GET", "/api/inspect"},
	{"POST", "/api/sessions"}, {"POST", "/api/sessions/1/stop"}, {"POST", "/api/sessions/1/fresh"},
	{"POST", "/api/sessions/1/close"}, {"POST", "/api/sessions/1/upload"}, {"GET", "/ws"},
	{"GET", "/api/settings/env"}, {"PUT", "/api/settings/env/X"}, {"DELETE", "/api/settings/env/X"},
	{"GET", "/api/settings/repo-prefix"}, {"PUT", "/api/settings/repo-prefix"}, {"DELETE", "/api/settings/repo-prefix"},
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/repos", s.repos)
	mux.HandleFunc("GET /api/inspect", s.inspect)
	mux.HandleFunc("POST /api/sessions", s.launch)
	mux.HandleFunc("POST /api/sessions/{slot}/upload", s.upload)
	mux.HandleFunc("POST /api/sessions/{slot}/{action}", s.action)
	mux.HandleFunc("GET /ws", s.ws)
	mux.HandleFunc("GET /api/settings/env", s.envList)
	mux.HandleFunc("PUT /api/settings/env/{name}", s.envSet)
	mux.HandleFunc("DELETE /api/settings/env/{name}", s.envSet)
	mux.HandleFunc("GET /api/settings/repo-prefix", s.prefixGet)
	mux.HandleFunc("PUT /api/settings/repo-prefix", s.prefixSet)
	mux.HandleFunc("DELETE /api/settings/repo-prefix", s.prefixSet)
	mux.HandleFunc("GET /", s.static)
	return s.Guard.Wrap(mux)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ve session.ValidationError
	switch {
	case errors.Is(err, session.ErrFull):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, session.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

type hello struct {
	Type        string          `json:"type,omitempty"`
	Host        string          `json:"host"`
	Login       string          `json:"login"`
	Version     string          `json:"version,omitempty"`
	MaxSessions int             `json:"max_sessions"`
	Slots       []*session.Info `json:"slots"`
}

func (s *Server) hello(r *http.Request) hello {
	return hello{Version: s.Version, Host: s.Host(), Login: auth.LoginFrom(r.Context()), MaxSessions: config.MaxSessions, Slots: s.M.Slots()}
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.hello(r)) }

// DefaultScanTimeout is how long a repo scan may run before it is stopped.
const DefaultScanTimeout = 10 * time.Second

// ScanTimeoutMsg is the 504 body and the text the launch list shows for it.
const ScanTimeoutMsg = "Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry."

// repos lists git repos under the saved repo_prefix. With no prefix it reads no folder.
func (s *Server) repos(w http.ResponseWriter, r *http.Request) {
	prefix, err := config.RepoPrefix(s.Home)
	if err != nil {
		writeErr(w, err)
		return
	}
	limit := s.ScanTimeout
	if limit <= 0 {
		limit = DefaultScanTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), limit)
	defer cancel()
	start := time.Now()
	res, err := repos.ScanPrefix(ctx, s.Home, prefix)
	if prefix != "" {
		s.Log.Info("repos_scan", "event", "repos_scan", "duration_ms", time.Since(start).Milliseconds(),
			"count", len(res.Repos), "timed_out", errors.Is(err, context.DeadlineExceeded))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": ScanTimeoutMsg})
		return
	}
	if err != nil {
		return // the caller went away
	}
	writeJSON(w, 200, struct {
		Repos   []repos.Repo `json:"repos"`
		Prefix  string       `json:"prefix"`
		Missing bool         `json:"missing,omitempty"`
	}{res.Repos, prefix, res.Missing})
}

func (s *Server) prefixGet(w http.ResponseWriter, r *http.Request) {
	prefix, err := config.RepoPrefix(s.Home)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"prefix": prefix})
}

// prefixSet saves (PUT {"prefix"}) or clears (DELETE) the repo folder. It needs
// the page's own Origin, as the other settings writes do.
func (s *Server) prefixSet(w http.ResponseWriter, r *http.Request) {
	if !auth.OriginOK(r) {
		http.Error(w, "Cross-origin request refused.", http.StatusForbidden)
		return
	}
	var value *string
	if r.Method == http.MethodPut {
		var body struct{ Prefix *string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Prefix == nil {
			writeJSON(w, 400, map[string]string{"error": "Enter a folder path."})
			return
		}
		value = body.Prefix
	}
	if err := config.SetRepoPrefix(s.Home, value); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	action := "set"
	if value == nil {
		action = "cleared"
	}
	s.Log.Info("settings_repo_prefix_changed", "event", "settings_repo_prefix_changed", "action", action, "login", auth.LoginFrom(r.Context()))
	s.prefixGet(w, r)
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request) {
	in, err := repos.Inspect(s.Home, r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, in)
}

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	var req session.LaunchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid request body."})
		return
	}
	info, err := s.M.Launch(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"slot": info.Slot, "info": info})
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil {
		writeErr(w, session.ErrNotFound)
		return
	}
	switch r.PathValue("action") {
	case "stop":
		if err := s.M.Stop(slot); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 202, map[string]any{})
	case "fresh":
		info, err := s.M.Fresh(slot)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"slot": info.Slot, "info": info})
	case "close":
		if err := s.M.Close(slot); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

// envList returns [env] names only; values are write-only.
func (s *Server) envList(w http.ResponseWriter, r *http.Request) {
	names, err := config.EnvNames(s.Home)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"names": names})
}

// envSet stores (PUT {"value"}) or removes (DELETE) one variable. Writing a
// secret requires the page's own Origin, not just the allowlist.
func (s *Server) envSet(w http.ResponseWriter, r *http.Request) {
	if !auth.OriginOK(r) {
		http.Error(w, "Cross-origin request refused.", http.StatusForbidden)
		return
	}
	name := r.PathValue("name")
	var value *string
	if r.Method == http.MethodPut {
		var body struct{ Value *string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || body.Value == nil {
			writeJSON(w, 400, map[string]string{"error": "Enter a value."})
			return
		}
		value = body.Value
	}
	if err := config.SetEnv(s.Home, name, value); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	action := "set"
	if value == nil {
		action = "removed"
	}
	s.Log.Info("settings_env_changed", "event", "settings_env_changed", "name", name, "action", action, "login", auth.LoginFrom(r.Context()))
	s.envList(w, r)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.Static == nil {
		http.Error(w, "Frontend not built.", 500)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.Static.Open(p); err == nil {
		st, _ := f.Stat()
		f.Close()
		if st != nil && !st.IsDir() {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, s.Static, p)
			return
		}
	}
	// The page has no client-side routes, so a missing path that looks like a file
	// (robots.txt, sitemap.xml, a stale chunk) is a 404, not the page.
	if strings.HasPrefix(p, "assets/") || path.Ext(p) != "" {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(s.Static, "index.html")
	if err != nil {
		http.Error(w, "Frontend not built. Run: npm --prefix web run build", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

// ---- WebSocket ----

type outFrame struct {
	typ  websocket.MessageType
	data []byte
}

type client struct {
	id     string
	out    chan outFrame
	events chan slotEvent
	ctx    context.Context
	cancel context.CancelFunc
}

type slotEvent struct {
	slot int
	info *session.Info
}

func (c *client) send(f outFrame) bool {
	select {
	case c.out <- f:
		return true
	case <-c.ctx.Done():
		return false
	default:
		c.cancel() // client too slow; it will reconnect and replay
		return false
	}
}

// SlotChanged is the manager's OnChange hook.
func (s *Server) SlotChanged(slot int, info *session.Info) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		select {
		case c.events <- slotEvent{slot, info}:
		default:
			c.cancel()
		}
	}
}

// CloseClients drops every WebSocket (listener going down).
func (s *Server) CloseClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		c.cancel()
	}
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // Origin checked by auth.Guard
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	var idb [8]byte
	rand.Read(idb[:])
	c := &client{id: hex.EncodeToString(idb[:]), out: make(chan outFrame, 4096), events: make(chan slotEvent, 64), ctx: ctx, cancel: cancel}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = map[*client]struct{}{}
	}
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		cancel()
		s.M.Forget(c.id)
		conn.CloseNow()
	}()

	h := s.hello(r)
	h.Type = "hello"
	b, _ := json.Marshal(h)
	c.send(outFrame{websocket.MessageText, b})

	subs := [config.MaxSessions]func(){}
	ids := [config.MaxSessions]string{}
	subscribe := func(slot int, id string) {
		if subs[slot-1] != nil {
			subs[slot-1]()
			subs[slot-1] = nil
		}
		ids[slot-1] = id
		if id == "" {
			return
		}
		snap, sub, cancelSub, err := s.M.Subscribe(slot)
		if err != nil {
			return
		}
		subs[slot-1] = cancelSub
		c.send(outFrame{websocket.MessageBinary, append([]byte{FrameReplay, byte(slot)}, snap...)})
		go func() {
			for chunk := range sub.C {
				if !c.send(outFrame{websocket.MessageBinary, append([]byte{FrameOutput, byte(slot)}, chunk...)}) {
					return
				}
			}
		}()
	}
	for i, inf := range h.Slots {
		if inf != nil {
			subscribe(i+1, inf.SessionID)
		}
	}

	// writer
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case f := <-c.out:
				wctx, wc := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, f.typ, f.data)
				wc()
				if err != nil {
					return
				}
			}
		}
	}()
	// slot events → text frames and resubscription (runs in this goroutine's helper)
	go func() {
		for {
			select {
			case <-ctx.Done():
				for _, f := range subs {
					if f != nil {
						f()
					}
				}
				return
			case ev := <-c.events:
				b, _ := json.Marshal(map[string]any{"type": "slot", "slot": ev.slot, "info": ev.info})
				c.send(outFrame{websocket.MessageText, b})
				id := ""
				if ev.info != nil {
					id = ev.info.SessionID
				}
				if id != ids[ev.slot-1] {
					subscribe(ev.slot, id)
				}
			}
		}
	}()
	// reader
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if len(data) >= 2 && data[0] == FrameOutput {
				_ = s.M.Input(int(data[1]), c.id, data[2:])
			}
		case websocket.MessageText:
			var m struct {
				Type       string
				Slot       int
				Rows, Cols uint16
			}
			if json.Unmarshal(data, &m) == nil && m.Type == "resize" {
				_ = s.M.Resize(m.Slot, c.id, m.Rows, m.Cols)
			}
		}
	}
}
