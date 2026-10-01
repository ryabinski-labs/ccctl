// Package repos finds git repositories under the saved repo_prefix for the launch
// form (docs/prd/repo-prefix.md).
package repos

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/ryabinski-labs/claude-code-controller/internal/session"
)

// MaxDepth is how many folder levels under a root are searched.
const MaxDepth = 3

type Repo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Display string `json:"display"`
}

var skip = map[string]bool{"node_modules": true, "vendor": true, "Library": true, "target": true, "dist": true, "build": true}

// isRepo is true for a main working tree (.git is a folder). Linked worktrees,
// including the ones ccctl creates, have a .git file and are not listed.
func isRepo(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && fi.IsDir()
}

// Inspection is GET /api/inspect.
type Inspection struct {
	Path            string `json:"path"`
	Git             bool   `json:"git"`
	WorktreeAllowed bool   `json:"worktree_allowed"`
	Note            string `json:"note"`
}

// Inspect validates path and reports whether a worktree is possible.
func Inspect(home, path string) (Inspection, error) {
	p, err := session.ValidatePath(expand(home, path))
	if err != nil {
		return Inspection{}, err
	}
	in := Inspection{Path: p, Git: session.IsGitRepo(p)}
	in.WorktreeAllowed = in.Git
	if !in.Git {
		in.Note = session.MsgNotGitRepo
	}
	return in, nil
}

func expand(home, p string) string {
	p = strings.TrimSpace(p)
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

type readDirFunc = func(string) ([]os.DirEntry, error)

var readDir atomic.Pointer[readDirFunc]

func init() {
	f := readDirFunc(os.ReadDir)
	readDir.Store(&f)
}

// SetReadDir replaces the folder reader the scan uses (tests count or block
// reads) and returns a function that puts the previous one back. A scan that
// timed out may still be running on its own goroutine, so the swap is atomic.
func SetReadDir(f func(string) ([]os.DirEntry, error)) (restore func()) {
	old := readDir.Load()
	readDir.Store(&f)
	return func() { readDir.Store(old) }
}

// Result is what a prefix scan found.
type Result struct {
	Repos   []Repo
	Missing bool // the prefix folder does not exist or is not a folder
}

// ScanPrefix lists git repos up to MaxDepth folder levels below prefix (a path
// that may start with ~). An empty prefix reads nothing. Repos are not descended
// into, and the prefix itself is listed when it is a repo. The walk runs on its
// own goroutine so a read that never returns (macOS waiting on a privacy prompt)
// cannot hold the caller past ctx; that goroutine ends when its read returns.
func ScanPrefix(ctx context.Context, home, prefix string) (Result, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return Result{Repos: []Repo{}}, nil
	}
	done := make(chan Result, 1)
	go func() { done <- scan(ctx, home, expand(home, prefix)) }()
	select {
	case r := <-done:
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return r, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func scan(ctx context.Context, home, root string) Result {
	out := Result{Repos: []Repo{}}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		out.Missing = true
		return out
	}
	add := func(p string) {
		d := p
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			d = "~/" + rel
		}
		out.Repos = append(out.Repos, Repo{Name: filepath.Base(p), Path: p, Display: d})
	}
	if isRepo(root) {
		add(root)
		return out
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > MaxDepth || ctx.Err() != nil {
			return
		}
		ents, err := (*readDir.Load())(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skip[e.Name()] {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if isRepo(p) {
				add(p)
				continue
			}
			walk(p, depth+1)
		}
	}
	walk(root, 1)
	sort.Slice(out.Repos, func(i, j int) bool { return out.Repos[i].Display < out.Repos[j].Display })
	return out
}
