package tdd

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/ryabinski-labs/claude-code-controller/internal/install"
)

// plistValue is a parsed plist node: a string, bool, []plistValue, or map.
type plistValue any

func parsePlist(t *testing.T, data []byte) map[string]plistValue {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	var parse func(start xml.StartElement) plistValue
	parse = func(start xml.StartElement) plistValue {
		switch start.Name.Local {
		case "true":
			_ = dec.Skip()
			return true
		case "false":
			_ = dec.Skip()
			return false
		case "string", "key", "integer":
			var s string
			if err := dec.DecodeElement(&s, &start); err != nil {
				t.Fatal(err)
			}
			return s
		case "array":
			var out []plistValue
			for {
				tok, err := dec.Token()
				if err != nil {
					t.Fatal(err)
				}
				switch el := tok.(type) {
				case xml.StartElement:
					out = append(out, parse(el))
				case xml.EndElement:
					return out
				}
			}
		case "dict":
			out := map[string]plistValue{}
			var key string
			for {
				tok, err := dec.Token()
				if err != nil {
					t.Fatal(err)
				}
				switch el := tok.(type) {
				case xml.StartElement:
					v := parse(el)
					if el.Name.Local == "key" {
						key = v.(string)
					} else {
						out[key] = v
					}
				case xml.EndElement:
					return out
				}
			}
		}
		t.Fatalf("unexpected plist element %s", start.Name.Local)
		return nil
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			t.Fatal("no dict in plist")
		}
		if err != nil {
			t.Fatal(err)
		}
		if el, ok := tok.(xml.StartElement); ok && el.Name.Local == "dict" {
			return parse(el).(map[string]plistValue)
		}
	}
}

func TestSc012ALaunchdAgentRendering(t *testing.T) {
	// SC-012-a (REQ-012, P1, unit)
	// Oracle: parsed plist ProgramArguments == [B, 'serve'] AND KeepAlive == true
	const b = "/Users/me/.ccctl/bin/ccctl"
	data, err := install.RenderLaunchdPlist(b, map[string]string{"PATH": "/usr/bin:/bin"}, "/Users/me/.ccctl/logs")
	if err != nil {
		t.Fatal(err)
	}
	d := parsePlist(t, data)
	args, _ := d["ProgramArguments"].([]plistValue)
	if len(args) != 2 || args[0] != b || args[1] != "serve" {
		t.Fatalf("ProgramArguments = %v, want [%s serve]", args, b)
	}
	if d["KeepAlive"] != true {
		t.Fatalf("KeepAlive = %v, want true", d["KeepAlive"])
	}
	if d["Label"] != install.Label {
		t.Fatalf("Label = %v", d["Label"])
	}
	env, _ := d["EnvironmentVariables"].(map[string]plistValue)
	if env["PATH"] != "/usr/bin:/bin" {
		t.Fatalf("EnvironmentVariables.PATH = %v", env["PATH"])
	}
}

func TestSc012BSystemdUnitRendering(t *testing.T) {
	// SC-012-b (REQ-012, P1, unit)
	// Oracle: unit has 'ExecStart=B serve' AND 'Restart=always'
	const b = "/home/me/.ccctl/bin/ccctl"
	unit := install.RenderSystemdUnit(b, map[string]string{"PATH": "/usr/bin"})
	lines := strings.Split(unit, "\n")
	has := func(want string) bool {
		for _, l := range lines {
			if strings.TrimSpace(l) == want {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"ExecStart=" + b + " serve", "Restart=always"} {
		if !has(want) {
			t.Fatalf("unit lacks line %q:\n%s", want, unit)
		}
	}
}

func TestSc012CReleaseAssetSelection(t *testing.T) {
	// SC-012-c (REQ-012, P1, unit)
	// Oracle: 4 supported pairs map to 4 distinct assets AND --version v1.2.3 selects tag v1.2.3 AND windows/amd64 returns an unsupported-platform error
	pairs := [][2]string{{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"}}
	rel := func(tag string) install.Release {
		r := install.Release{TagName: tag}
		for _, p := range pairs {
			r.Assets = append(r.Assets, install.Asset{Name: install.AssetName(p[0], p[1]), URL: tag + "/" + p[0] + "/" + p[1]})
		}
		r.Assets = append(r.Assets, install.Asset{Name: "checksums.txt", URL: tag + "/sums"})
		return r
	}
	seen := map[string]bool{}
	for _, p := range pairs {
		a, err := install.SelectAsset(p[0], p[1], "", rel("v1.3.0"))
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		if a.Name != "ccctl_"+p[0]+"_"+p[1]+".tar.gz" {
			t.Fatalf("%v: asset %s", p, a.Name)
		}
		seen[a.URL] = true
	}
	if len(seen) != 4 {
		t.Fatalf("distinct assets = %d, want 4", len(seen))
	}
	if got := install.ReleaseAPIPath("v1.2.3"); !strings.HasSuffix(got, "/releases/tags/v1.2.3") {
		t.Fatalf("versioned API path = %s", got)
	}
	if got := install.ReleaseAPIPath(""); !strings.HasSuffix(got, "/releases/latest") {
		t.Fatalf("latest API path = %s", got)
	}
	a, err := install.SelectAsset("linux", "amd64", "v1.2.3", rel("v1.2.3"))
	if err != nil || !strings.HasPrefix(a.URL, "v1.2.3/") {
		t.Fatalf("--version v1.2.3: asset %v err %v", a, err)
	}
	if _, err := install.SelectAsset("linux", "amd64", "v1.2.3", rel("v1.3.0")); err == nil {
		t.Fatal("mismatched tag accepted")
	}
	_, err = install.SelectAsset("windows", "amd64", "", rel("v1.3.0"))
	if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("windows/amd64 err = %v, want unsupported platform", err)
	}
}

func TestSc012DInstallRecordsClaudePathAndSecuresConfig(t *testing.T) {
	// SC-012-d (REQ-012, P1, unit)
	// Oracle: config claude_path == '/h/.local/bin/claude' AND config file mode == 0600
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".ccctl", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("port = 7700\nroots = [\"~/src\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lookPath := func(name string) (string, error) {
		if name != "claude" {
			return "", errors.New("not found")
		}
		return "/h/.local/bin/claude", nil
	}
	if err := install.RecordClaudePath(cfgPath, lookPath); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		ClaudePath string   `toml:"claude_path"`
		Port       int      `toml:"port"`
		Roots      []string `toml:"roots"`
	}
	if _, err := toml.DecodeFile(cfgPath, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClaudePath != "/h/.local/bin/claude" {
		t.Fatalf("claude_path = %q", cfg.ClaudePath)
	}
	if cfg.Port != 7700 || len(cfg.Roots) != 1 || cfg.Roots[0] != "~/src" {
		t.Fatalf("other keys not preserved: %+v", cfg)
	}
	st, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", st.Mode().Perm())
	}
}
