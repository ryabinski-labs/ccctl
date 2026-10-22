// Green-phase install scenarios from tdd/opencode-support.tdd.yaml (REQ-006).
package tdd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/ryabinski-labs/ccctl/internal/install"
	"github.com/ryabinski-labs/ccctl/internal/opencode"
)

func lookOpenCode(path string) func(string) (string, error) {
	return func(name string) (string, error) {
		if name != "opencode" {
			return "", errors.New("not found")
		}
		return path, nil
	}
}

// SC-006-a
func TestSc006ARecordsOpenCodePath(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("claude_path = \"/keep/me\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := install.RecordOpenCodePath(cfg, lookOpenCode("/opt/homebrew/bin/opencode"))
	if err != nil || p != "/opt/homebrew/bin/opencode" {
		t.Fatalf("path = %q err = %v", p, err)
	}
	var got struct {
		ClaudePath   string `toml:"claude_path"`
		OpenCodePath string `toml:"opencode_path"`
	}
	if _, err := toml.DecodeFile(cfg, &got); err != nil {
		t.Fatal(err)
	}
	if got.ClaudePath != "/keep/me" || got.OpenCodePath != "/opt/homebrew/bin/opencode" {
		t.Fatalf("config = %+v", got)
	}
	fi, _ := os.Stat(cfg)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

// SC-006-b
func TestSc006BMissingOpenCodeIsOptional(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("claude_path = \"/keep/me\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := install.RecordOpenCodePath(cfg, func(string) (string, error) { return "", errors.New("not found") })
	if err != nil || p != "" {
		t.Fatalf("path = %q err = %v", p, err)
	}
	var got map[string]any
	if _, err := toml.DecodeFile(cfg, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["opencode_path"]; ok {
		t.Fatal("opencode_path was written")
	}
	if got["claude_path"] != "/keep/me" {
		t.Fatalf("config = %v", got)
	}
}

// SC-006-c
func TestSc006CPluginWritten(t *testing.T) {
	dir := t.TempDir()
	p, err := opencode.WritePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "plugins", "ccctl.js"); p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{opencode.Marker, "permission.asked", "session.idle", "CCCTL_SOCK", "CCCTL_SLOT"} {
		if !strings.Contains(string(b), s) {
			t.Errorf("plugin is missing %q", s)
		}
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm()&0o444 == 0 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

// SC-006-d
func TestSc006DForeignPluginKept(t *testing.T) {
	dir := t.TempDir()
	p := opencode.PluginPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := opencode.WritePlugin(dir)
	if !errors.Is(err, opencode.ErrPluginConflict) {
		t.Fatalf("err = %v", err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "user-owned\n" {
		t.Fatalf("file = %q", b)
	}
}

// SC-006-e
func TestSc006ERemoveOnlyMarkedPlugin(t *testing.T) {
	owned := t.TempDir()
	marked, err := opencode.WritePlugin(owned)
	if err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	p := opencode.PluginPath(foreign)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := opencode.RemovePlugin(owned); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marked plugin still exists: %v", err)
	}
	if err := opencode.RemovePlugin(foreign); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(b, []byte("user-owned\n")) {
		t.Fatalf("foreign plugin changed: %q %v", b, err)
	}
}

// SC-006-d (install wiring)
func TestSc006DInstallWarnsOnPluginConflict(t *testing.T) {
	home := t.TempDir()
	configDir := t.TempDir()
	p := opencode.PluginPath(configDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := install.Run(context.Background(), install.Options{
		Home: home, GOOS: "darwin", Local: true,
		LookPath: func(name string) (string, error) {
			if name == "opencode" {
				return "/opt/homebrew/bin/opencode", nil
			}
			return "/opt/homebrew/bin/claude", nil
		},
		Env: func(k string) string {
			switch k {
			case "OPENCODE_CONFIG_DIR":
				return configDir
			case "PATH":
				return "/usr/bin:/bin"
			case "USER":
				return "tester"
			}
			return ""
		},
		Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "opencode_plugin_conflict") {
		t.Fatalf("install output = %q", out.String())
	}
	b, _ := os.ReadFile(p)
	if string(b) != "user-owned\n" {
		t.Fatalf("plugin was overwritten: %q", b)
	}
	var cfg struct {
		OpenCodePath      string `toml:"opencode_path"`
		OpenCodeConfigDir string `toml:"opencode_config_dir"`
	}
	if _, err := toml.DecodeFile(install.ConfigPath(home), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodePath != "/opt/homebrew/bin/opencode" || cfg.OpenCodeConfigDir != configDir {
		t.Fatalf("config = %+v", cfg)
	}
}

// SC-006-d (configured directory)
func TestSc006DInstallUsesConfiguredOpenCodeDir(t *testing.T) {
	home := t.TempDir()
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(install.ConfigPath(home)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(install.ConfigPath(home), []byte("opencode_config_dir = \""+configDir+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := opencode.PluginPath(configDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := install.Run(context.Background(), install.Options{
		Home: home, GOOS: "darwin", Local: true,
		LookPath: func(name string) (string, error) {
			if name == "opencode" {
				return "/opt/homebrew/bin/opencode", nil
			}
			return "/opt/homebrew/bin/claude", nil
		},
		Env: func(k string) string {
			if k == "PATH" {
				return "/usr/bin:/bin"
			}
			return ""
		},
		Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "opencode_plugin_conflict") {
		t.Fatalf("install output = %q", out.String())
	}
	if b, _ := os.ReadFile(p); string(b) != "user-owned\n" {
		t.Fatalf("plugin was overwritten: %q", b)
	}
}
