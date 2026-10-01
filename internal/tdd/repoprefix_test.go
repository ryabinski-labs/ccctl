// Repository folder prefix scenarios (tdd/repo-prefix.tdd.yaml, docs/prd/repo-prefix.md).
package tdd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/repos"
	"github.com/ryabinski-labs/claude-code-controller/internal/server"
	tu "github.com/ryabinski-labs/claude-code-controller/internal/testutil"
)

const prefixTimeoutMsg = "Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry."

func mkRepo(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func strp(s string) *string { return &s }

func writeCfg(t testing.TB, home, body string) string {
	t.Helper()
	p := config.Path(home)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileBytes(p string) []byte { b, _ := os.ReadFile(p); return b }

// countReads makes the scanner's directory reads countable for the test's lifetime.
func countReads(t testing.TB) *atomic.Int64 {
	var n atomic.Int64
	t.Cleanup(repos.SetReadDir(func(p string) ([]os.DirEntry, error) { n.Add(1); return os.ReadDir(p) }))
	return &n
}

// blockReads makes every directory read wait until the returned release runs (also on cleanup).
func blockReads(t testing.TB) (release func()) {
	ch := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(ch) }) }
	restore := repos.SetReadDir(func(p string) ([]os.DirEntry, error) { <-ch; return os.ReadDir(p) })
	t.Cleanup(func() { release(); restore() })
	return release
}

type reposResp struct {
	Repos   []repos.Repo `json:"repos"`
	Prefix  string       `json:"prefix"`
	Missing bool         `json:"missing"`
}

func getRepos(t testing.TB, h *tu.H) (int, reposResp, string) {
	t.Helper()
	code, b := h.Do("GET", "/api/repos", nil)
	var r reposResp
	json.Unmarshal(b, &r)
	return code, r, string(b)
}

func repoPaths(r reposResp) []string {
	out := []string{}
	for _, x := range r.Repos {
		out = append(out, x.Path)
	}
	sort.Strings(out)
	return out
}

// SC-pfx-set-a
func TestSCpfxSetASavingAndClearingEditsOneKey(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "projects"), 0o755)
	p := writeCfg(t, home, "port = 7700\n[env]\nGEMINI_API_KEY = \"k\"\n")
	if err := config.SetRepoPrefix(home, strp("~/projects")); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(home)
	if err != nil || c.RepoPrefix != "~/projects" || c.Port != 7700 || c.Env["GEMINI_API_KEY"] != "k" {
		t.Fatalf("after save: %+v, %v", c, err)
	}
	if err := config.SetRepoPrefix(home, nil); err != nil {
		t.Fatal(err)
	}
	raw := string(fileBytes(p))
	c, err = config.Load(home)
	if err != nil || strings.Contains(raw, "repo_prefix") || c.RepoPrefix != "" || c.Port != 7700 || c.Env["GEMINI_API_KEY"] != "k" {
		t.Fatalf("after clear: %+v, %v\n%s", c, err, raw)
	}
}

// SC-pfx-set-b
func TestSCpfxSetBFolderThatIsNotAnExistingAbsoluteFolderIsRefused(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "projects"), 0o755)
	os.WriteFile(filepath.Join(home, "notes.txt"), []byte("x"), 0o644)
	p := writeCfg(t, home, "repo_prefix = \"~/projects\"\n")
	before := fileBytes(p)
	for _, typed := range []string{"/no/such/folder", "~/notes.txt", "src"} {
		err := config.SetRepoPrefix(home, strp(typed))
		want := fmt.Sprintf("Folder %s does not exist or is not a folder.", typed)
		if err == nil || err.Error() != want {
			t.Errorf("save %q: err = %v; want %q", typed, err, want)
		}
		if !reflect.DeepEqual(fileBytes(p), before) {
			t.Errorf("save %q changed config.toml", typed)
		}
	}
}

// SC-pfx-set-c
func TestSCpfxSetCPuttingThePrefixLimitsTheLaunchList(t *testing.T) {
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		for _, d := range []string{"projects/a", "projects/b/c", "other/d"} {
			mkRepo(t, filepath.Join(home, d))
		}
	}})
	code, body := h.Do("PUT", "/api/settings/repo-prefix", map[string]string{"prefix": "~/projects"})
	if code != 200 || strings.TrimSpace(string(body)) != `{"prefix":"~/projects"}` {
		t.Fatalf("PUT = %d %s", code, body)
	}
	if code, body := h.Do("GET", "/api/settings/repo-prefix", nil); code != 200 || strings.TrimSpace(string(body)) != `{"prefix":"~/projects"}` {
		t.Fatalf("GET setting = %d %s", code, body)
	}
	code, r, raw := getRepos(t, h)
	want := []string{filepath.Join(h.Home, "projects/a"), filepath.Join(h.Home, "projects/b/c")}
	if code != 200 || r.Prefix != "~/projects" || !reflect.DeepEqual(repoPaths(r), want) {
		t.Fatalf("GET /api/repos = %d %s; want paths %v", code, raw, want)
	}
}

// SC-pfx-set-e
func TestSCpfxSetEDeletingThePrefixReturnsToTheUnsetState(t *testing.T) {
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		mkRepo(t, filepath.Join(home, "projects/a"))
		mkRepo(t, filepath.Join(home, "projects/b"))
		writeCfg(t, home, "repo_prefix = \"~/projects\"\n")
	}})
	code, body := h.Do("DELETE", "/api/settings/repo-prefix", nil)
	if code != 200 || strings.TrimSpace(string(body)) != `{"prefix":""}` {
		t.Fatalf("DELETE = %d %s", code, body)
	}
	if raw := fileBytes(config.Path(h.Home)); strings.Contains(string(raw), "repo_prefix") {
		t.Fatalf("config still has the key:\n%s", raw)
	}
	if code, _, raw := getRepos(t, h); code != 200 || strings.TrimSpace(raw) != `{"prefix":"","repos":[]}` && strings.TrimSpace(raw) != `{"repos":[],"prefix":""}` {
		t.Fatalf("GET /api/repos = %d %s", code, raw)
	}
}

// SC-pfx-auth-a
func TestSCpfxAuthAUnlistedLoginIsRefusedOnTheRepoFolderRoutes(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	h.TS.Set(asOther)
	p := config.Path(h.Home)
	before := fileBytes(p)
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if code, _ := h.Do(m, "/api/settings/repo-prefix", map[string]string{"prefix": "~"}); code != 403 {
			t.Errorf("%s = %d", m, code)
		}
		found := false
		for _, r := range server.Routes {
			found = found || r.Method == m && r.Path == "/api/settings/repo-prefix"
		}
		if !found {
			t.Errorf("server.Routes lacks %s /api/settings/repo-prefix", m)
		}
	}
	if !reflect.DeepEqual(fileBytes(p), before) {
		t.Error("config.toml changed")
	}
}

// SC-pfx-auth-b
func TestSCpfxAuthBCrossOriginWriteIsRefused(t *testing.T) {
	h := tu.Start(t, tu.Opts{Setup: func(home string) { os.MkdirAll(filepath.Join(home, "projects"), 0o755) }})
	p := config.Path(h.Home)
	// The route must exist and accept the page's own Origin, or a refusal proves nothing.
	if code, body := h.Do("PUT", "/api/settings/repo-prefix", map[string]string{"prefix": "~/projects"}); code != 200 {
		t.Fatalf("same-origin PUT = %d %s", code, body)
	}
	before := fileBytes(p)
	for _, m := range []string{"PUT", "DELETE"} {
		req, _ := http.NewRequest(m, h.URL()+"/api/settings/repo-prefix", strings.NewReader(`{"prefix":"~/projects"}`))
		req.Header.Set("Origin", "http://evil.example")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		b.Write(buf[:n])
		resp.Body.Close()
		if resp.StatusCode != 403 || !strings.Contains(b.String(), "Cross-origin request refused.") {
			t.Errorf("%s = %d %q", m, resp.StatusCode, b.String())
		}
	}
	if !reflect.DeepEqual(fileBytes(p), before) {
		t.Error("config.toml changed")
	}
}

// SC-pfx-noscan-a
func TestSCpfxNoscanAScannerReadsNothingWithoutAPrefix(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"projects/a", "Documents/b", "direct"} {
		mkRepo(t, filepath.Join(home, d))
	}
	n := countReads(t)
	res, err := repos.ScanPrefix(context.Background(), home, "")
	if err != nil || len(res.Repos) != 0 || res.Missing || n.Load() != 0 {
		t.Fatalf("result = %+v, err = %v, reads = %d", res, err, n.Load())
	}
}

// SC-pfx-noscan-b
func TestSCpfxNoscanBOldRootsAndHomeChildrenAreIgnored(t *testing.T) {
	n := countReads(t)
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		mkRepo(t, filepath.Join(home, "Documents", "folio"))
		mkRepo(t, filepath.Join(home, "direct"))
		writeCfg(t, home, fmt.Sprintf("roots = [%q]\n", filepath.Join(home, "Documents")))
	}})
	code, r, raw := getRepos(t, h)
	if code != 200 || r.Prefix != "" || len(r.Repos) != 0 || !strings.Contains(raw, `"repos":[]`) || n.Load() != 0 {
		t.Fatalf("GET /api/repos = %d %s; reads = %d", code, raw, n.Load())
	}
}

// SC-pfx-timeout-a
func TestSCpfxTimeoutAScannerGivesUpWhenItsContextExpires(t *testing.T) {
	home := t.TempDir()
	mkRepo(t, filepath.Join(home, "projects", "a"))
	release := blockReads(t)
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := repos.ScanPrefix(ctx, home, filepath.Join(home, "projects"))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("err = %v after %v", err, time.Since(start))
	}
	release()
	tu.Eventually(t, time.Second, func() bool { return runtime.NumGoroutine() <= base }, "the blocked scan goroutine never ended")
}

// SC-pfx-timeout-b
func TestSCpfxTimeoutBSlowScanReturns504WithTheExplanation(t *testing.T) {
	blockReads(t)
	h := tu.Start(t, tu.Opts{ScanTimeout: 100 * time.Millisecond, Setup: func(home string) {
		mkRepo(t, filepath.Join(home, "projects", "a"))
		writeCfg(t, home, "repo_prefix = \"~/projects\"\n")
	}})
	code, b := h.Do("GET", "/api/repos", nil)
	var e struct{ Error string }
	json.Unmarshal(b, &e)
	if code != 504 || e.Error != prefixTimeoutMsg {
		t.Fatalf("GET /api/repos = %d %s", code, b)
	}
	tu.Eventually(t, 2*time.Second, func() bool {
		for _, ev := range h.Log.Events("repos_scan") {
			if d, _ := ev["duration_ms"].(float64); ev["timed_out"] == true && d >= 100 {
				return true
			}
		}
		return false
	}, "no repos_scan log line with timed_out true and duration_ms >= 100")
}

func warnLines(h *tu.H) int {
	n := 0
	for _, l := range strings.Split(h.Log.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && (m["level"] == "WARN" || m["level"] == "ERROR") {
			n++
		}
	}
	return n
}

// SC-pfx-empty-a
func TestSCpfxEmptyAEmptyAndDeletedPrefixFoldersGiveDistinctResponses(t *testing.T) {
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		os.MkdirAll(filepath.Join(home, "empty"), 0o755)
		writeCfg(t, home, "repo_prefix = \"~/empty\"\n")
	}})
	warns := warnLines(h)
	code, r, raw := getRepos(t, h)
	if code != 200 || r.Prefix != "~/empty" || len(r.Repos) != 0 || r.Missing || !strings.Contains(raw, `"repos":[]`) {
		t.Fatalf("empty folder: %d %s", code, raw)
	}
	os.Remove(filepath.Join(h.Home, "empty"))
	code, r, raw = getRepos(t, h)
	if code != 200 || r.Prefix != "~/empty" || len(r.Repos) != 0 || !r.Missing {
		t.Fatalf("deleted folder: %d %s", code, raw)
	}
	if got := warnLines(h); got != warns {
		t.Errorf("%d warning or error log lines were added:\n%s", got-warns, h.Log.String())
	}
}

// SC-pfx-scan-a
func TestSCpfxScanADepthAndEntryRulesUnderThePrefix(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, "P")
	for _, d := range []string{"a", "x/y/b", "x/y/z/c", "a/inner"} {
		mkRepo(t, filepath.Join(p, d))
	}
	os.MkdirAll(filepath.Join(p, "wt"), 0o755)
	os.WriteFile(filepath.Join(p, "wt", ".git"), []byte("gitdir: ../a/.git/worktrees/wt\n"), 0o644)
	res, err := repos.ScanPrefix(context.Background(), home, p)
	got := map[string]bool{}
	for _, r := range res.Repos {
		got[r.Path] = true
	}
	want := map[string]bool{filepath.Join(p, "a"): true, filepath.Join(p, "x/y/b"): true}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("scan = %v, %v; want %v", got, err, want)
	}
	self := filepath.Join(home, "self")
	mkRepo(t, self)
	mkRepo(t, filepath.Join(self, "child"))
	res, err = repos.ScanPrefix(context.Background(), home, self)
	if err != nil || len(res.Repos) != 1 || res.Repos[0].Path != self {
		t.Fatalf("prefix that is a repo: %+v, %v", res, err)
	}
}

// SC-pfx-docs-a
func TestSCpfxDocsAReadmeDocumentsRepoPrefix(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(tu.RepoRoot(), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "| `roots` |") {
			t.Errorf("README still lists roots as a setting: %s", l)
		}
		if strings.HasPrefix(l, "| `repo_prefix` |") {
			row = l
		}
	}
	if row == "" || !regexp.MustCompile(`(?i)unset`).MatchString(row) || !strings.Contains(row, "3 levels") || !strings.Contains(row, "roots") {
		t.Fatalf("README repo_prefix row = %q; want it to say unset, 3 levels and that roots is ignored", row)
	}
}

// SC-pfx-nfr-a
func TestSCpfxNfrAHundredSavesKeepTheModeAndTheOtherKeys(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "projects"), 0o755)
	p := writeCfg(t, home, "port = 7700\nallowed_logins = [\"a@example\"]\n[env]\nGEMINI_API_KEY = \"k\"\n")
	for i := 0; i < 100; i++ {
		var err error
		if i%2 == 0 {
			err = config.SetRepoPrefix(home, strp("~/projects"))
		} else {
			err = config.SetRepoPrefix(home, nil)
		}
		if err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		fi, _ := os.Stat(p)
		c, lerr := config.Load(home)
		if fi.Mode().Perm() != 0o600 || lerr != nil || c.Port != 7700 || !reflect.DeepEqual(c.AllowedLogins, []string{"a@example"}) || c.Env["GEMINI_API_KEY"] != "k" {
			t.Fatalf("write %d: mode %v, %+v, %v", i, fi.Mode().Perm(), c, lerr)
		}
	}
}

// SC-pfx-nfr-b
func TestSCpfxNfrBScanningFiftyReposIsUnderASecondAtP95(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, "P")
	for i := 0; i < 50; i++ {
		mkRepo(t, filepath.Join(p, fmt.Sprintf("g%d", i%5), fmt.Sprintf("r%d", i)))
	}
	var ds []time.Duration
	for i := 0; i < 20; i++ {
		start := time.Now()
		res, err := repos.ScanPrefix(context.Background(), home, p)
		ds = append(ds, time.Since(start))
		if err != nil || len(res.Repos) != 50 {
			t.Fatalf("run %d: %d repos, %v", i, len(res.Repos), err)
		}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	if !raceEnabled && ds[18] >= time.Second {
		t.Fatalf("p95 = %v", ds[18])
	}
}
