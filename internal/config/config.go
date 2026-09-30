// Package config loads ~/.ccctl/config.toml and applies the spec defaults (§12).
package config

import (
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// MaxSessions is fixed at 4 (DL-010).
const MaxSessions = 4

// DefaultPort is the controller port (A-005).
const DefaultPort = 7681

type Config struct {
	Roots         []string `toml:"roots"`
	Port          int      `toml:"port"`
	AllowedLogins []string `toml:"allowed_logins"`
	ClaudePath    string   `toml:"claude_path"`
	TailscalePath string   `toml:"tailscale_path"`
	// Env is added to every Claude session's environment (for example
	// GEMINI_API_KEY). It is re-read at each launch, so edits apply to the next
	// session without a restart. Values are never logged.
	Env map[string]string `toml:"env"`
}

// Dir is the ccctl data folder under home (A-010).
func Dir(home string) string { return filepath.Join(home, ".ccctl") }

// Path is the config file path under home.
func Path(home string) string { return filepath.Join(Dir(home), "config.toml") }

// Load reads the config file if it exists and fills defaults.
func Load(home string) (Config, error) {
	var c Config
	if _, err := toml.DecodeFile(Path(home), &c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return c, err
	}
	c.ApplyDefaults(home, exec.LookPath)
	return c, nil
}

// ApplyDefaults fills unset fields (A-020, A-022) and expands ~ in paths.
func (c *Config) ApplyDefaults(home string, lookPath func(string) (string, error)) {
	if len(c.Roots) == 0 {
		c.Roots = []string{"~/projects", "~/Documents"}
	}
	for i, r := range c.Roots {
		c.Roots[i] = Expand(home, r)
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.ClaudePath == "" {
		if p, err := lookPath("claude"); err == nil {
			c.ClaudePath = p
		} else {
			c.ClaudePath = filepath.Join(home, ".local", "bin", "claude")
		}
	}
	c.ClaudePath = Expand(home, c.ClaudePath)
	c.TailscalePath = Expand(home, c.TailscalePath)
}

// Expand replaces a leading ~ with home.
func Expand(home, p string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
