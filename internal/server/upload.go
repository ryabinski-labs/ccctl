package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/auth"
	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/session"
)

const (
	// MaxUpload is the largest file one upload may carry.
	MaxUpload = 25 << 20
	// MaxUploadDir caps the total size of the uploads folder.
	MaxUploadDir = 1 << 30
	// UploadKeep is how long an upload is kept before a later upload removes it.
	UploadKeep = 14 * 24 * time.Hour
	// uploadReadLimit is how long one upload body may take to arrive.
	uploadReadLimit = 5 * time.Minute
)

// ourUpload matches the names upload() creates, so pruning never touches other files.
var ourUpload = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{6}-`)

// pruneUploads removes uploads older than UploadKeep and returns the size of what is left.
func pruneUploads(dir string, now time.Time) int64 {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range ents {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if ourUpload.MatchString(e.Name()) && now.Sub(info.ModTime()) > UploadKeep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		total += info.Size()
	}
	return total
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// uploadName turns a browser-supplied file name into a short, shell-safe one.
func uploadName(raw string) string {
	base := raw
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.Trim(unsafeName.ReplaceAllString(base, "_"), "._")
	if len(base) > 80 {
		ext := filepath.Ext(base)
		if len(ext) > 12 {
			ext = ""
		}
		base = base[:80-len(ext)] + ext
	}
	if base == "" {
		return "file"
	}
	return base
}

// upload stores one file from the page under ~/.ccctl/uploads and returns its
// absolute path, so the page can paste that path into the session. The session
// runs as the same user, so Claude Code can read it. Each upload also removes
// ccctl's own uploads older than UploadKeep.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if !auth.OriginOK(r) {
		http.Error(w, "Cross-origin request refused.", http.StatusForbidden)
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil {
		writeErr(w, session.ErrNotFound)
		return
	}
	info := s.M.Slots()
	if slot < 1 || slot > len(info) || info[slot-1] == nil {
		writeErr(w, session.ErrNotFound)
		return
	}
	if !info[slot-1].State.Active() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "That session has ended."})
		return
	}
	dir := filepath.Join(config.Dir(s.Home), "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, err)
		return
	}
	if pruneUploads(dir, time.Now()) >= MaxUploadDir {
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "The upload folder on the host is full. Remove old files from ~/.ccctl/uploads."})
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadLimit))
	var rnd [3]byte
	_, _ = rand.Read(rnd[:])
	name := time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(rnd[:]) + "-" + uploadName(r.URL.Query().Get("name"))
	full := filepath.Join(dir, name)
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, err := io.Copy(f, http.MaxBytesReader(w, r.Body, MaxUpload))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(full)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "That file is over 25 MB."})
			return
		}
		writeErr(w, err)
		return
	}
	s.Log.Info("upload", "event", "upload", "slot", slot, "bytes", n, "login", auth.LoginFrom(r.Context()))
	writeJSON(w, http.StatusCreated, map[string]any{"path": full, "size": n})
}
