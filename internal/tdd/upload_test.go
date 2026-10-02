// Uploading a file from the page into a session's host.
package tdd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryabinski-labs/ccctl/internal/server"
	tu "github.com/ryabinski-labs/ccctl/internal/testutil"
)

func postUpload(t *testing.T, h *tu.H, slot, name string, body []byte, origin string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", h.URL()+"/api/sessions/"+slot+"/upload?name="+name, bytes.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestUploadStoresFileForTheHostUser(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 1)
	payload := []byte("\x89PNG not really")
	code, b := postUpload(t, h, "1", "..%2F..%2Fmy%20shot%20(1).png", payload, h.URL())
	if code != 201 {
		t.Fatalf("status %d %s", code, b)
	}
	var out struct {
		Path string
		Size int
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(h.Home, ".ccctl", "uploads") + string(filepath.Separator); !strings.HasPrefix(out.Path, want) {
		t.Fatalf("path %q is outside %q", out.Path, want)
	}
	if strings.ContainsAny(filepath.Base(out.Path), " ()") || !strings.HasSuffix(out.Path, "my_shot_1_.png") && !strings.HasSuffix(out.Path, "my_shot_1.png") {
		t.Fatalf("name not sanitized: %q", out.Path)
	}
	got, err := os.ReadFile(out.Path)
	if err != nil || !bytes.Equal(got, payload) || out.Size != len(payload) {
		t.Fatalf("stored %q (%v), size %d", got, err, out.Size)
	}
	if st, _ := os.Stat(out.Path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
}

func TestUploadRefusals(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 1)
	if code, _ := postUpload(t, h, "1", "a.txt", []byte("x"), ""); code != 403 {
		t.Errorf("no Origin: %d, want 403", code)
	}
	if code, _ := postUpload(t, h, "1", "a.txt", []byte("x"), "http://evil.example"); code != 403 {
		t.Errorf("foreign Origin: %d, want 403", code)
	}
	if code, _ := postUpload(t, h, "3", "a.txt", []byte("x"), h.URL()); code != 404 {
		t.Errorf("empty slot: %d, want 404", code)
	}
	if code, _ := postUpload(t, h, "1", "big.bin", make([]byte, 25<<20+1), h.URL()); code != 413 {
		t.Errorf("oversize: %d, want 413", code)
	}
	if ents, _ := os.ReadDir(filepath.Join(h.Home, ".ccctl", "uploads")); len(ents) != 0 {
		t.Errorf("refused uploads left %d files", len(ents))
	}
}

func TestUploadRefusedForEndedSession(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 1)
	if code, b := h.Do("POST", "/api/sessions/1/stop", nil); code != 202 {
		t.Fatalf("stop: %d %s", code, b)
	}
	tu.Eventually(t, 10*time.Second, func() bool {
		s := h.Slot(1)
		return s != nil && !s.State.Active()
	}, "session never ended")
	if code, _ := postUpload(t, h, "1", "late.txt", []byte("x"), h.URL()); code != 409 {
		t.Errorf("upload to ended session: %d, want 409", code)
	}
	if ents, _ := os.ReadDir(filepath.Join(h.Home, ".ccctl", "uploads")); len(ents) != 0 {
		t.Errorf("refused upload left %d files", len(ents))
	}
}

func TestUploadPrunesOldFilesButNotOthers(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 1)
	dir := filepath.Join(h.Home, ".ccctl", "uploads")
	os.MkdirAll(dir, 0o700)
	old := filepath.Join(dir, "20200101-000000-aabbcc-old.png")
	stranger := filepath.Join(dir, "notes-i-keep.txt") // not ours: wrong name shape
	for _, f := range []string{old, stranger} {
		os.WriteFile(f, []byte("x"), 0o600)
		os.Chtimes(f, time.Now().Add(-30*24*time.Hour), time.Now().Add(-30*24*time.Hour))
	}
	if code, b := postUpload(t, h, "1", "new.txt", []byte("y"), h.URL()); code != 201 {
		t.Fatalf("upload: %d %s", code, b)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old upload was not pruned (%v)", err)
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("a file ccctl did not create was removed: %v", err)
	}
}

func TestUploadRefusedWhenFolderIsFull(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 1)
	dir := filepath.Join(h.Home, ".ccctl", "uploads")
	os.MkdirAll(dir, 0o700)
	f, _ := os.Create(filepath.Join(dir, time.Now().Format("20060102-150405")+"-aabbcc-big.bin"))
	f.Truncate(server.MaxUploadDir + 1) // sparse: no real disk used
	f.Close()
	if code, _ := postUpload(t, h, "1", "a.txt", []byte("x"), h.URL()); code != 507 {
		t.Errorf("upload into a full folder: %d, want 507", code)
	}
}
