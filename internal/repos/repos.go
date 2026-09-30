// Package repos finds git repositories for the launch form (REQ-003, A-020).
package repos

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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

func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Scan lists repos up to MaxDepth levels under each existing root plus direct
// children of home. Missing roots are skipped. Repos are not descended into.
func Scan(home string, roots []string) []Repo {
	seen := map[string]bool{}
	var out []Repo
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			d := p
			if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
				d = "~/" + rel
			}
			out = append(out, Repo{Name: filepath.Base(p), Path: p, Display: d})
		}
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > MaxDepth {
			return
		}
		ents, err := os.ReadDir(dir)
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
	for _, r := range roots {
		if fi, err := os.Stat(r); err == nil && fi.IsDir() {
			walk(r, 1)
		}
	}
	if ents, err := os.ReadDir(home); err == nil {
		for _, e := range ents {
			p := filepath.Join(home, e.Name())
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && isRepo(p) {
				add(p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Display < out[j].Display })
	return out
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
