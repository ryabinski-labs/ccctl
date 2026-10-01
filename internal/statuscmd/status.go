// Package statuscmd implements `ccctl status` (exit 0 serving, 1 not serving, 3 Tailscale not running).
package statuscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/hook"
	"github.com/ryabinski-labs/claude-code-controller/internal/tailscale"
)

const (
	ExitOK           = 0
	ExitNotServing   = 1
	ExitTailscaleOff = 3
)

// Run prints the controller URL or the reason it is unreachable.
func Run(ctx context.Context, ts tailscale.Client, port int, sock string, w io.Writer) int {
	st, err := ts.Status(ctx)
	if st.BackendState == "NoCLI" && err != nil {
		fmt.Fprintln(w, err.Error())
		return ExitTailscaleOff
	}
	if st.BackendState != "Running" {
		fmt.Fprintln(w, tailscale.NotRunningMessage)
		return ExitTailscaleOff
	}
	url := "http://" + net.JoinHostPort(st.IPv4, strconv.Itoa(port))
	resp, err := hook.UnixClient(sock, 2*time.Second).Get("http://ccctl/status")
	if err != nil {
		fmt.Fprintf(w, "%s\nccctl is not running on this host.\n", url)
		return ExitNotServing
	}
	defer resp.Body.Close()
	var rep struct{ URL, Message string }
	json.NewDecoder(resp.Body).Decode(&rep)
	if rep.URL == "" {
		fmt.Fprintf(w, "%s\nccctl is running but not serving yet. %s\n", url, rep.Message)
		return ExitNotServing
	}
	fmt.Fprintln(w, rep.URL)
	return ExitOK
}
