// Terminal I/O, replay, and mirroring scenarios (REQ-004, REQ-011, REQ-013).
package tdd

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	tu "github.com/ryabinski-labs/claude-code-controller/internal/testutil"
)

// SC-004-b
func TestSc004BInputBytesRouteExactlyToOnePty(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 4)
	ws := h.Dial()
	for s := 1; s <= 4; s++ {
		ws.WaitOutput(s, "FAKE_CLAUDE", 5*time.Second)
	}
	for _, in := range []string{"hello\r", "\x1b", "\x03", "\x1b[A"} {
		ws.Input(2, in)
	}
	want := "68656c6c6f0d" + "1b" + "03" + "1b5b41"
	tu.Eventually(t, 2*time.Second, func() bool { return hex.EncodeToString(h.InputLog(2)) == want }, "slot 2 bytes = "+hex.EncodeToString(h.InputLog(2)))
	time.Sleep(500 * time.Millisecond)
	for _, s := range []int{1, 3, 4} {
		if b := h.InputLog(s); len(b) != 0 {
			t.Errorf("slot %d read %q", s, b)
		}
	}
}

var lineRe = regexp.MustCompile(`slot=(\d) line=(\d+)`)

// SC-004-c
func TestSc004COutputFanOutKeepsPerSlotOrder(t *testing.T) {
	h := tu.Start(t, tu.Opts{Mode: "lines=1000"})
	ws := h.Dial()
	launchN(h, 4)
	for s := 1; s <= 4; s++ {
		ws.WaitOutput(s, fmt.Sprintf("slot=%d line=1000\r\n", s), 10*time.Second)
	}
	for s := 1; s <= 4; s++ {
		var nums []int
		for _, m := range lineRe.FindAllStringSubmatch(ws.Output(s), -1) {
			if m[1] != strconv.Itoa(s) {
				t.Fatalf("slot %d frame carries slot %s's line", s, m[1])
			}
			n, _ := strconv.Atoi(m[2])
			nums = append(nums, n)
		}
		if len(nums) != 1000 || !sort.IntsAreSorted(nums) || nums[0] != 1 || nums[999] != 1000 {
			t.Fatalf("slot %d: %d lines", s, len(nums))
		}
	}
}

// SC-004-d
func TestSc004DResizeFrameSetsPtyWindowSize(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 3)
	ws := h.Dial()
	ws.WaitOutput(3, "FAKE_CLAUDE", 5*time.Second)
	ws.Resize(3, 50, 200)
	tu.Eventually(t, 2*time.Second, func() bool {
		r, c, err := h.App.M.PTYSize(3)
		return err == nil && r == 50 && c == 200
	}, "PTY size never became 50x200")
}

var numRe = regexp.MustCompile(`line=(\d+)`)

// SC-011-b
func TestSc011BReplayPrecedesLiveOutput(t *testing.T) {
	h := tu.Start(t, tu.Opts{Mode: "lines=100"})
	d := filepath.Join(h.Root, "r")
	os.MkdirAll(d, 0o755)
	h.Launch("replay", d, false, "")
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 1 }, "not started")
	time.Sleep(300 * time.Millisecond) // lines 1-100 printed with no client connected
	ws := h.Dial()
	ws.WaitOutput(1, "line=100\r\n", 3*time.Second)
	ws.Input(1, "print 101 101\r")
	ws.WaitOutput(1, "line=101\r\n", 3*time.Second)
	var got []int
	for _, m := range numRe.FindAllStringSubmatch(ws.Output(1), -1) {
		n, _ := strconv.Atoi(m[1])
		got = append(got, n)
	}
	for i, n := range got {
		if n != i+1 {
			t.Fatalf("line order broken at %d: %v", i, got)
		}
	}
	if len(got) != 101 {
		t.Fatalf("got %d lines", len(got))
	}
}

// SC-011-c
func TestSc011CTwoTabsMirrorSizeFollowsLastInput(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	d := filepath.Join(h.Root, "m")
	os.MkdirAll(d, 0o755)
	h.Launch("mirror", d, false, "")
	a, b := h.Dial(), h.Dial()
	a.WaitOutput(1, "FAKE_CLAUDE", 5*time.Second)
	a.Resize(1, 40, 120)
	b.Resize(1, 30, 100)
	size := func() string { r, c, _ := h.App.M.PTYSize(1); return fmt.Sprintf("%dx%d", r, c) }
	a.Input(1, "from-a\r")
	a.WaitOutput(1, "ECHO from-a", 3*time.Second) // A's input processed before B types
	tu.Eventually(t, 2*time.Second, func() bool { return size() == "40x120" }, "size after A = "+size())
	b.Input(1, "from-b\r")
	tu.Eventually(t, 2*time.Second, func() bool { return size() == "30x100" }, "size after B = "+size())
	a.WaitOutput(1, "ECHO from-b", 3*time.Second)
	b.WaitOutput(1, "ECHO from-b", 3*time.Second)
	if a.Output(1) != b.Output(1) {
		t.Fatalf("tabs differ:\nA=%q\nB=%q", a.Output(1), b.Output(1))
	}
}

var tsRe = regexp.MustCompile(`TS (\d+) (\d+)\r\n`)

// SC-013-a
func TestSc013AControllerAddedOutputLatency(t *testing.T) {
	if raceEnabled {
		t.Skip("latency budget applies to normal builds; CI runs this test without -race")
	}
	h := tu.Start(t, tu.Opts{Mode: "ticker=2500"})
	ws := h.Dial()
	launchN(h, 4)
	tu.Eventually(t, 30*time.Second, func() bool {
		for s := 1; s <= 4; s++ {
			if !strings.Contains(ws.Output(s), " 2500\r\n") {
				return false
			}
		}
		return true
	}, "tickers did not finish")
	var d []time.Duration
	for _, f := range ws.FramesCopy() {
		for _, m := range tsRe.FindAllStringSubmatch(string(f.Data), -1) {
			ns, _ := strconv.ParseInt(m[1], 10, 64)
			d = append(d, f.At.Sub(time.Unix(0, ns)))
		}
	}
	if len(d) < 10000 {
		t.Fatalf("only %d samples", len(d))
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	p95 := d[len(d)*95/100]
	t.Logf("samples=%d p50=%v p95=%v max=%v", len(d), d[len(d)/2], p95, d[len(d)-1])
	if p95 > 20*time.Millisecond {
		t.Fatalf("p95 = %v > 20ms", p95)
	}
}

// SC-013-d
func TestSc013DBinarySizePerPlatform(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles 4 binaries")
	}
	dir := t.TempDir()
	for _, p := range [][2]string{{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"}} {
		out := filepath.Join(dir, p[0]+"_"+p[1])
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "./cmd/ccctl")
		cmd.Dir = tu.RepoRoot()
		cmd.Env = append(os.Environ(), "GOOS="+p[0], "GOARCH="+p[1], "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", p, err, b)
		}
		fi, _ := os.Stat(out)
		t.Logf("%s/%s: %d bytes", p[0], p[1], fi.Size())
		if fi.Size() > 31457280 {
			t.Errorf("%v is %d bytes", p, fi.Size())
		}
	}
	_ = runtime.GOOS
}
