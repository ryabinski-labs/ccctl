// Documentation scenario from tdd/opencode-support.tdd.yaml (REQ-009).
package tdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SC-009-a
func TestSc009ADocsCoverBothAgents(t *testing.T) {
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	readme := read("README.md")
	for _, s := range []string{"OpenCode", "opencode_path", "2.1.3", "Resume by ID is Claude Code only"} {
		if !strings.Contains(readme, s) {
			t.Errorf("README is missing %q", s)
		}
	}
	protocol := read("docs/design/protocol.md")
	for _, s := range []string{"agent", "opencode"} {
		if !strings.Contains(protocol, s) {
			t.Errorf("protocol.md is missing %q", s)
		}
	}
}
