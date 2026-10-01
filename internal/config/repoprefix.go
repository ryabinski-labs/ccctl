package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// RepoPrefix returns the saved repo_prefix as typed, or "" when it is unset.
// It reads only that key, so the launch list does not pay for a full Load.
func RepoPrefix(home string) (string, error) {
	var c struct {
		RepoPrefix string `toml:"repo_prefix"`
	}
	if _, err := toml.DecodeFile(Path(home), &c); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return strings.TrimSpace(c.RepoPrefix), nil
}

// ValidateRepoPrefix checks that typed is an existing absolute folder once ~ is expanded.
func ValidateRepoPrefix(home, typed string) error {
	if typed == "" {
		return errors.New("Enter a folder path.")
	}
	p := Expand(home, typed)
	if fi, err := os.Stat(p); !filepath.IsAbs(p) || err != nil || !fi.IsDir() {
		return fmt.Errorf("Folder %s does not exist or is not a folder.", typed)
	}
	return nil
}

// SetRepoPrefix saves (value != nil) or clears (value == nil) repo_prefix,
// keeping every other key. A value that is not an existing absolute folder is
// refused and nothing is written.
func SetRepoPrefix(home string, value *string) error {
	var typed string
	if value != nil {
		typed = strings.TrimSpace(*value)
		if err := ValidateRepoPrefix(home, typed); err != nil {
			return err
		}
	}
	return editConfig(home, func(raw map[string]any) {
		if value == nil {
			delete(raw, "repo_prefix")
		} else {
			raw["repo_prefix"] = typed
		}
	})
}
