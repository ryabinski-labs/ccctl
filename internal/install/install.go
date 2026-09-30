// Package install puts ccctl on a host as a background service: a launchd user
// agent on macOS or a systemd user unit on Linux (spec REQ-012, DL-014).
package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// Label is the launchd agent label.
	Label = "com.ryabinski-labs.ccctl"
	// Repo is the GitHub repository that publishes releases.
	Repo = "ryabinski-labs/claude-code-controller"
	// UnitName is the systemd user unit name.
	UnitName = "ccctl.service"
)

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"` // API URL; fetch with Accept: application/octet-stream
}

// Release is the subset of the GitHub release JSON ccctl needs.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// AssetName is the GoReleaser archive name for a platform.
func AssetName(goos, goarch string) string {
	return fmt.Sprintf("ccctl_%s_%s.tar.gz", goos, goarch)
}

// ReleaseAPIPath is the GitHub API path for the latest release, or for tag
// version when version is not empty.
func ReleaseAPIPath(version string) string {
	if version == "" {
		return "/repos/" + Repo + "/releases/latest"
	}
	return "/repos/" + Repo + "/releases/tags/" + version
}

// SelectAsset picks the archive for goos/goarch from rel. When version is not
// empty, rel must be that tag.
func SelectAsset(goos, goarch, version string, rel Release) (Asset, error) {
	switch goos + "/" + goarch {
	case "darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64":
	default:
		return Asset{}, fmt.Errorf("unsupported platform %s/%s: ccctl ships darwin and linux on arm64 and amd64", goos, goarch)
	}
	if version != "" && rel.TagName != version {
		return Asset{}, fmt.Errorf("release %s does not match requested version %s", rel.TagName, version)
	}
	want := AssetName(goos, goarch)
	for _, a := range rel.Assets {
		if a.Name == want {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no asset %s", rel.TagName, want)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// RenderLaunchdPlist renders the macOS user agent that runs "binPath serve".
func RenderLaunchdPlist(binPath string, env map[string]string, logDir string) ([]byte, error) {
	if binPath == "" {
		return nil, errors.New("binary path is empty")
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(binPath) + `</string>
		<string>serve</string>
	</array>
	<key>KeepAlive</key>
	<true/>
	<key>RunAtLoad</key>
	<true/>
`)
	if len(env) > 0 {
		b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, k := range sortedKeys(env) {
			b.WriteString("\t\t<key>" + xmlEscape(k) + "</key>\n\t\t<string>" + xmlEscape(env[k]) + "</string>\n")
		}
		b.WriteString("\t</dict>\n")
	}
	if logDir != "" {
		b.WriteString("\t<key>StandardOutPath</key>\n\t<string>" + xmlEscape(filepath.Join(logDir, "launchd.out.log")) + "</string>\n")
		b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>" + xmlEscape(filepath.Join(logDir, "launchd.err.log")) + "</string>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String()), nil
}

// RenderSystemdUnit renders the Linux user unit that runs "binPath serve".
func RenderSystemdUnit(binPath string, env map[string]string) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=ccctl Claude Code controller\nAfter=network-online.target\n\n[Service]\n")
	b.WriteString("ExecStart=" + binPath + " serve\n")
	b.WriteString("Restart=always\nRestartSec=2\n")
	for _, k := range sortedKeys(env) {
		b.WriteString(fmt.Sprintf("Environment=%q\n", k+"="+env[k]))
	}
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RecordClaudePath writes the absolute path of the claude found on PATH into
// the config file as claude_path, keeping every other key. Services do not load
// the user's shell PATH, so the path is recorded at install time (A-022).
func RecordClaudePath(configPath string, lookPath func(string) (string, error)) error {
	p, err := lookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found on PATH: install Claude Code first: %w", err)
	}
	if !filepath.IsAbs(p) {
		if p, err = filepath.Abs(p); err != nil {
			return err
		}
	}
	return setConfigKey(configPath, "claude_path", p)
}

func setConfigKey(configPath, key string, value any) error {
	cfg := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil {
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return fmt.Errorf("read %s: %w", configPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg[key] = value
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(configPath, buf.Bytes(), 0o600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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
	return os.Rename(tmp.Name(), path)
}

// Runner runs an external command. Tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs commands with os/exec.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Options configures Run and Uninstall.
type Options struct {
	Home     string // default: os.UserHomeDir()
	GOOS     string // default: runtime.GOOS
	GOARCH   string // default: runtime.GOARCH
	Version  string // "" = latest release
	Local    bool   // copy the running executable instead of downloading
	Run      Runner
	LookPath func(string) (string, error)
	HTTP     *http.Client
	APIBase  string // default https://api.github.com
	Token    string // default $GITHUB_TOKEN, else `gh auth token`
	Env      func(string) string
	UID      int
	Out      io.Writer
}

func (o *Options) defaults() error {
	if o.Home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		o.Home = h
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Run == nil {
		o.Run = ExecRunner
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.APIBase == "" {
		o.APIBase = "https://api.github.com"
	}
	if o.Env == nil {
		o.Env = os.Getenv
	}
	if o.UID == 0 {
		o.UID = os.Getuid()
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	return nil
}

// Paths are where install puts things under a home folder.
func BinPath(home string) string    { return filepath.Join(home, ".ccctl", "bin", "ccctl") }
func ConfigPath(home string) string { return filepath.Join(home, ".ccctl", "config.toml") }
func LogDir(home string) string     { return filepath.Join(home, ".ccctl", "logs") }
func PlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}
func UnitPath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", UnitName)
}

// Run installs the binary and the service, then starts the service.
func Run(ctx context.Context, o Options) error {
	if err := o.defaults(); err != nil {
		return err
	}
	if o.GOOS != "darwin" && o.GOOS != "linux" {
		return fmt.Errorf("unsupported platform %s/%s", o.GOOS, o.GOARCH)
	}
	bin := BinPath(o.Home)
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(LogDir(o.Home), 0o700); err != nil {
		return err
	}
	if o.Local {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(bin, data, 0o755); err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "Installed %s from %s\n", bin, self)
	} else {
		tag, err := download(ctx, &o, bin)
		if err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "Installed ccctl %s to %s\n", tag, bin)
	}
	if err := RecordClaudePath(ConfigPath(o.Home), o.LookPath); err != nil {
		return err
	}
	env := map[string]string{"PATH": o.Env("PATH"), "HOME": o.Home}
	switch o.GOOS {
	case "darwin":
		plist, err := RenderLaunchdPlist(bin, env, LogDir(o.Home))
		if err != nil {
			return err
		}
		p := PlistPath(o.Home)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := writeFileAtomic(p, plist, 0o644); err != nil {
			return err
		}
		domain := fmt.Sprintf("gui/%d", o.UID)
		_, _ = o.Run(ctx, "launchctl", "bootout", domain+"/"+Label)
		if out, err := o.Run(ctx, "launchctl", "bootstrap", domain, p); err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
		}
		fmt.Fprintf(o.Out, "Loaded launchd agent %s\n", p)
	case "linux":
		p := UnitPath(o.Home)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := writeFileAtomic(p, []byte(RenderSystemdUnit(bin, env)), 0o644); err != nil {
			return err
		}
		if out, err := o.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %v: %s", err, out)
		}
		if out, err := o.Run(ctx, "systemctl", "--user", "enable", "--now", "ccctl"); err != nil {
			return fmt.Errorf("systemctl enable: %v: %s", err, out)
		}
		fmt.Fprintf(o.Out, "Enabled systemd user unit %s\n", p)
		fmt.Fprintf(o.Out, "To keep ccctl running while you are logged out, run: loginctl enable-linger %s\n", o.Env("USER"))
	}
	return nil
}

// Uninstall stops and removes the service definition only. It never touches
// ~/.ccctl (state, config, logs) or any worktree.
func Uninstall(ctx context.Context, o Options) error {
	if err := o.defaults(); err != nil {
		return err
	}
	switch o.GOOS {
	case "darwin":
		_, _ = o.Run(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", o.UID, Label))
		if err := os.Remove(PlistPath(o.Home)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	case "linux":
		_, _ = o.Run(ctx, "systemctl", "--user", "disable", "--now", "ccctl")
		if err := os.Remove(UnitPath(o.Home)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_, _ = o.Run(ctx, "systemctl", "--user", "daemon-reload")
	default:
		return fmt.Errorf("unsupported platform %s", o.GOOS)
	}
	fmt.Fprintln(o.Out, "Service removed. ~/.ccctl and all worktrees were kept.")
	return nil
}

func token(ctx context.Context, o *Options) (string, error) {
	if o.Token != "" {
		return o.Token, nil
	}
	if t := o.Env("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	out, err := o.Run(ctx, "gh", "auth", "token")
	if err != nil {
		return "", errors.New("no GitHub token: set GITHUB_TOKEN or log in with `gh auth login` (the release repo is private)")
	}
	return strings.TrimSpace(string(out)), nil
}

func download(ctx context.Context, o *Options, dest string) (string, error) {
	tok, err := token(ctx, o)
	if err != nil {
		return "", err
	}
	get := func(url, accept string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := o.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
		}
		return resp, nil
	}
	resp, err := get(o.APIBase+ReleaseAPIPath(o.Version), "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var rel Release
	err = json.NewDecoder(resp.Body).Decode(&rel)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	asset, err := SelectAsset(o.GOOS, o.GOARCH, o.Version, rel)
	if err != nil {
		return "", err
	}
	resp, err = get(asset.URL, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := extractBinary(resp.Body)
	if err != nil {
		return "", err
	}
	return rel.TagName, writeFileAtomic(dest, data, 0o755)
}

func extractBinary(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("archive has no ccctl binary")
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "ccctl" {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}
