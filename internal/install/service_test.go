package install

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

// An upgrade must restart a unit that is already running: `enable --now` alone keeps the old binary.
func TestLinuxInstallRestartsTheService(t *testing.T) {
	var calls []string
	o := Options{
		Home:     t.TempDir(),
		GOOS:     "linux",
		Local:    true,
		LookPath: func(string) (string, error) { return "/usr/bin/claude", nil },
		Env:      func(string) string { return "" },
		Out:      io.Discard,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		},
	}
	if err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "systemctl --user restart ccctl") {
		t.Fatalf("service not restarted; calls: %q", calls)
	}
}
