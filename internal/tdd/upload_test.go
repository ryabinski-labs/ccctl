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

	tu "github.com/ryabinski-labs/claude-code-controller/internal/testutil"
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
