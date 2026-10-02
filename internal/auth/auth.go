// Package auth gates every request on the Tailscale WhoIs allowlist (DL-006),
// the Host header (DNS-rebinding guard), and the Origin header (R-003).
package auth

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ryabinski-labs/ccctl/internal/logx"
)

// Authorize reports whether login is on the allowlist. An empty login (WhoIs failure) is denied.
func Authorize(allow []string, login string) bool {
	return login != "" && slices.Contains(allow, login)
}

// DeniedMessage is the §5 403 text.
func DeniedMessage(login string) string {
	if login == "" {
		login = "unknown"
	}
	return fmt.Sprintf("Not authorized: %s is not on this controller's allowlist.", login)
}

// WhoIsFunc resolves a remote address to a tailnet login.
type WhoIsFunc func(ctx context.Context, addr string) (string, error)

// Cache memoizes WhoIs per remote IP for TTL (60 s by spec).
type Cache struct {
	Lookup WhoIsFunc
	TTL    time.Duration
	Now    func() time.Time
	mu     sync.Mutex
	m      map[string]cacheEntry
}

type cacheEntry struct {
	login string
	at    time.Time
}

func NewCache(lookup WhoIsFunc) *Cache {
	return &Cache{Lookup: lookup, TTL: 60 * time.Second, Now: time.Now}
}

// Login returns the login for addr (ip:port); the cache key is the IP.
func (c *Cache) Login(ctx context.Context, addr string) (string, error) {
	ip := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		ip = h
	}
	now := c.Now()
	c.mu.Lock()
	if e, ok := c.m[ip]; ok && now.Sub(e.at) < c.TTL {
		c.mu.Unlock()
		return e.login, nil
	}
	c.mu.Unlock()
	login, err := c.Lookup(ctx, addr)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]cacheEntry{}
	}
	c.m[ip] = cacheEntry{login, now}
	c.mu.Unlock()
	return login, nil
}

type ctxKey struct{}

// LoginFrom returns the authorized login stored on the request context.
func LoginFrom(ctx context.Context) string { s, _ := ctx.Value(ctxKey{}).(string); return s }

// Guard is the middleware.
type Guard struct {
	Cache *Cache
	Allow func() []string
	// Hosts returns the accepted Host header values (host or host:port).
	Hosts func() []string
	Log   *slog.Logger
}

func (g *Guard) hostOK(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	for _, a := range g.Hosts() {
		if strings.EqualFold(a, host) || strings.EqualFold(a, h) {
			return true
		}
	}
	return false
}

// OriginOK reports whether origin equals the controller's own origin http://<Host>.
func OriginOK(r *http.Request) bool {
	o := r.Header.Get("Origin")
	return o != "" && strings.EqualFold(o, "http://"+r.Host)
}

func (g *Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login, err := g.Cache.Login(r.Context(), r.RemoteAddr)
		if err != nil || !Authorize(g.Allow(), login) {
			logx.Event(g.Log, "auth_denied", "login", login, "node", r.RemoteAddr, "path", r.URL.Path)
			deny(w, DeniedMessage(login))
			return
		}
		if !g.hostOK(r.Host) {
			http.Error(w, "Unknown host.", http.StatusForbidden)
			return
		}
		isWS := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		if isWS && !OriginOK(r) {
			http.Error(w, "Cross-origin WebSocket refused.", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != "" && !OriginOK(r) {
			http.Error(w, "Cross-origin request refused.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, login)))
	})
}

func deny(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, deniedPage, html.EscapeString(msg))
}

const deniedPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Not authorized · ccctl</title>
<style>:root{color-scheme:light}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#F5F3EE;color:#1C1B18;font:16px/1.5 "IBM Plex Sans",system-ui,sans-serif}
main{max-width:34rem;padding:2rem;border:1px solid #DAD6CC;border-top:3px solid #B42318;background:#fff}h1{font-size:1.1rem;margin:0 0 .5rem}p{margin:0;color:#55524B}</style></head>
<body><main><h1>ccctl</h1><p>%s</p></main></body></html>`
