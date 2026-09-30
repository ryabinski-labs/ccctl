package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// IsGitRepo reports whether dir is inside a git work tree.
func IsGitRepo(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// WorktreePlan is where a task's worktree goes: <top>-wt-<task> beside the repo top level.
type WorktreePlan struct {
	Top, Path, Branch, Cwd string
}

// PlanWorktree computes the sibling worktree folder and branch for dir and task.
func PlanWorktree(dir, task string) (WorktreePlan, error) {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return WorktreePlan{}, fmt.Errorf("git: %s", top)
	}
	top, _ = filepath.EvalSymlinks(top)
	realDir, _ := filepath.EvalSymlinks(dir)
	rel, err := filepath.Rel(top, realDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = "."
	}
	p := WorktreePlan{Top: top, Path: filepath.Join(filepath.Dir(top), filepath.Base(top)+"-wt-"+task), Branch: "ctl/" + task}
	p.Cwd = filepath.Join(p.Path, rel)
	return p, nil
}

// Check refuses when the folder or branch already exists.
func (p WorktreePlan) Check(task string) error {
	if _, err := os.Lstat(p.Path); err == nil {
		return errors.New(MsgWorktreeExists(task))
	}
	if _, err := git(p.Top, "rev-parse", "--verify", "--quiet", "refs/heads/"+p.Branch); err == nil {
		return errors.New(MsgWorktreeExists(task))
	}
	return nil
}

// Create runs `git worktree add -b ctl/TASK <path> HEAD`.
func (p WorktreePlan) Create() error {
	if out, err := git(p.Top, "worktree", "add", "-b", p.Branch, p.Path, "HEAD"); err != nil {
		return fmt.Errorf("git worktree add failed: %s", out)
	}
	return nil
}
