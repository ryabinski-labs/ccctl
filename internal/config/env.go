package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

var writeMu sync.Mutex

// ReservedEnv are names the controller sets itself or strips on purpose.
func ReservedEnv(name string) bool {
	return name == "TERM" || name == "COLORTERM" || name == "HOME" || name == "PATH" || strings.HasPrefix(name, "CCCTL_") ||
		name == "CLAUDECODE" || name == "CLAUDE_CODE_ENTRYPOINT" || name == "CLAUDE_CODE_CHILD_SESSION"
}

// ValidEnvName is a POSIX-style variable name.
func ValidEnvName(k string) bool {
	if k == "" || len(k) > 128 {
		return false
	}
	for i, c := range k {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// MaxEnvValue bounds one value (OAuth tokens and API keys are far smaller).
const MaxEnvValue = 16 << 10

// EnvNames lists the [env] names in config.toml, sorted. Values are never returned.
func EnvNames(home string) ([]string, error) {
	c, err := Load(home)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.Env))
	for k := range c.Env {
		names = append(names, k)
	}
	sort.Strings(names)
	return names, nil
}

// SetEnv sets (value != nil) or removes (value == nil) one [env] entry,
// preserving every other key, and writes config.toml atomically with mode 0600.
func SetEnv(home, name string, value *string) error {
	if !ValidEnvName(name) {
		return fmt.Errorf("Variable names use letters, digits and _, and do not start with a digit.")
	}
	if ReservedEnv(name) {
		return fmt.Errorf("%s is set by ccctl and cannot be changed here.", name)
	}
	if value != nil && (len(*value) > MaxEnvValue || strings.ContainsRune(*value, 0)) {
		return fmt.Errorf("Value must be at most %d bytes of text.", MaxEnvValue)
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	p := Path(home)
	raw := map[string]any{}
	if _, err := toml.DecodeFile(p, &raw); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	env, _ := raw["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	if value == nil {
		delete(env, name)
	} else {
		env[name] = *value
	}
	raw["env"] = env
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
