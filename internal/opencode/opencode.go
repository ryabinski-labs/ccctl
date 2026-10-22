// Package opencode provides the OpenCode integration assets: the needs-input
// plugin and the install/uninstall helpers for its config directory.
package opencode

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Marker identifies a plugin file owned by ccctl. uninstall removes a file only
// when this marker is present.
const Marker = "ccctl-opencode-plugin"

// PluginName is the file name inside the OpenCode plugins directory.
const PluginName = "ccctl.js"

// ErrPluginConflict means a file exists at the plugin path but ccctl did not
// write it; install leaves it untouched.
var ErrPluginConflict = errors.New("an OpenCode plugin already exists at this path and was not written by ccctl")

//go:embed plugin.js
var pluginSource []byte

// Source returns the plugin file contents.
func Source() []byte { return append([]byte(nil), pluginSource...) }

// IsOurs reports whether content carries the ccctl marker.
func IsOurs(content []byte) bool { return bytes.Contains(content, []byte(Marker)) }

// DefaultConfigDir is ~/.config/opencode.
func DefaultConfigDir(home string) string { return filepath.Join(home, ".config", "opencode") }

// ConfigDir resolves OPENCODE_CONFIG_DIR when set, else the default. A leading
// ~ is expanded against home because the service does not load a shell.
func ConfigDir(home string, getenv func(string) string) string {
	return ResolveConfigDir(home, "", getenv)
}

// ResolveConfigDir resolves the OpenCode config directory from the controller
// config value and OPENCODE_CONFIG_DIR, in that order, with the default last.
func ResolveConfigDir(home, configured string, getenv func(string) string) string {
	d := strings.TrimSpace(configured)
	if getenv != nil {
		if env := strings.TrimSpace(getenv("OPENCODE_CONFIG_DIR")); env != "" {
			d = env
		}
	}
	if d == "" {
		return DefaultConfigDir(home)
	}
	if d == "~" {
		return home
	}
	if strings.HasPrefix(d, "~/") {
		return filepath.Join(home, d[2:])
	}
	return d
}

// PluginPath is the plugin file path inside configDir.
func PluginPath(configDir string) string { return filepath.Join(configDir, "plugins", PluginName) }

// WritePlugin writes or updates the ccctl plugin. An existing file without the
// ccctl marker returns ErrPluginConflict and is left byte-for-byte unchanged.
func WritePlugin(configDir string) (string, error) {
	p := PluginPath(configDir)
	if old, err := os.ReadFile(p); err == nil && !IsOurs(old) {
		return "", fmt.Errorf("%w: %s", ErrPluginConflict, p)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, pluginSource, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// RemovePlugin deletes the plugin only when it carries the ccctl marker and
// does nothing when the file is absent or user-owned.
func RemovePlugin(configDir string) error {
	p := PluginPath(configDir)
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !IsOurs(b) {
		return nil
	}
	return os.Remove(p)
}
