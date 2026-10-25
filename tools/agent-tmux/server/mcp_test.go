package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// msgWriter is Serve's stdout in the concurrent tests. It records any Write that overlaps another or is not
// exactly one JSON message with its newline, and hands each message to the test.
type msgWriter struct {
	active  atomic.Int32
	hold    time.Duration
	mu      sync.Mutex
	errs    []string
	replies chan map[string]any
}

func (w *msgWriter) fail(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.errs = append(w.errs, msg)
}

func (w *msgWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) != 1 {
		w.fail("two writes overlapped")
	}
	defer w.active.Add(-1)
	// Hold the write open briefly, so writers that are not serialised would overlap.
	time.Sleep(w.hold)
	if len(p) == 0 || p[len(p)-1] != '\n' || bytes.Count(p, []byte("\n")) != 1 {
		w.fail(fmt.Sprintf("write is not exactly one message: %q", p))
		return len(p), nil
	}
	var m map[string]any
	if err := json.Unmarshal(p, &m); err != nil {
		w.fail(fmt.Sprintf("write is not JSON: %q", p))
		return len(p), nil
	}
	w.replies <- m
	return len(p), nil
}

// live is one Serve running on a pipe, for tests that need calls in flight at the same time.
type live struct {
	in   *io.PipeWriter
	out  *msgWriter
	done chan error
}

func serveLive(t *testing.T, s *Server) *live {
	t.Helper()
	pr, pw := io.Pipe()
	l := &live{in: pw, out: &msgWriter{hold: 2 * time.Millisecond, replies: make(chan map[string]any, 256)}, done: make(chan error, 1)}
	go func() { l.done <- s.Serve(pr, l.out) }()
	t.Cleanup(func() {
		pw.Close()
		select {
		case <-l.done:
		case <-time.After(5 * time.Second):
			t.Errorf("Serve still running at cleanup")
		}
		l.out.mu.Lock()
		defer l.out.mu.Unlock()
		for _, e := range l.out.errs {
			t.Errorf("stdout: %s", e)
		}
	})
	return l
}

// send writes the lines in one write. It fails instead of hanging if Serve has stopped reading stdin.
func (l *live) send(t *testing.T, lines ...string) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(l.in, strings.Join(lines, "\n")+"\n")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write to Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Serve stopped reading stdin")
	}
}

// next returns the next reply, failing if none arrives in time.
func (l *live) next(t *testing.T, within time.Duration) map[string]any {
	t.Helper()
	select {
	case r := <-l.out.replies:
		return r
	case <-time.After(within):
		t.Fatalf("no reply within %v", within)
		return nil
	}
}

// close ends stdin and waits for Serve to return, failing if it takes longer than within.
func (l *live) close(t *testing.T, within time.Duration) {
	t.Helper()
	l.in.Close()
	select {
	case err := <-l.done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
		l.done <- nil // for the cleanup
	case <-time.After(within):
		t.Fatalf("Serve did not return within %v of EOF", within)
	}
}

// rest drains the replies already written; call it after close.
func (l *live) rest() []map[string]any {
	var out []map[string]any
	for {
		select {
		case r := <-l.out.replies:
			out = append(out, r)
		default:
			return out
		}
	}
}

func cancelNote(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":%d,"reason":"test"}}`, id)
}

func replyID(r map[string]any) int {
	id, _ := r["id"].(float64)
	return int(id)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !cond(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
	}
}

// waitForLog polls the audit log until it holds want, so promptness is measured, not assumed.
func waitForLog(t *testing.T, path, want string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(path); strings.Contains(string(b), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("audit log lacks %q after %v:\n%s", want, within, b)
}

const waitForever = `{"pane":"%5","pattern":"never-printed","timeout":300}`

func TestReadLoopDoesNotBlockOnWait(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever), `{"jsonrpc":"2.0","id":2,"method":"ping"}`, call(3, "tmux_list", ""))
	got := map[int]bool{}
	for range 2 {
		got[replyID(l.next(t, 2*time.Second))] = true
	}
	if !got[2] || !got[3] {
		t.Fatalf("ping and list must be answered while the wait runs, got %v", got)
	}
	l.send(t, cancelNote(1))
	l.close(t, 2*time.Second)
}

func TestCancelledWaitStopsPromptly(t *testing.T) {
	h := newLayoutHarness(t)
	h.s.waitPoll = time.Hour // the cancel must end the sleep between polls, not wait for the next one
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever))
	waitUntil(t, "the wait polls", func() bool { return h.f.count("capture-pane") > 0 })
	start := time.Now()
	l.send(t, cancelNote(1))
	waitForLog(t, h.logPath, "tool=tmux_wait target=%5 result=cancelled", 2*time.Second)
	if d := time.Since(start); d > time.Second {
		t.Errorf("wait took %v to stop after notifications/cancelled", d)
	}
	// The read loop still serves after the cancellation.
	l.send(t, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if r := l.next(t, 2*time.Second); replyID(r) != 2 {
		t.Fatalf("want the ping reply, got %v", r)
	}
	l.close(t, 2*time.Second)
	if rs := l.rest(); len(rs) != 0 {
		t.Errorf("a cancelled request gets no reply, got %v", rs)
	}
}

// A cancel whose requestId is spelt in another case, or given twice, is ignored rather than decoded loosely.
func TestCancelKeysExact(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever))
	waitUntil(t, "the wait polls", func() bool { return h.f.count("capture-pane") > 0 })
	l.send(t,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2,"RequestId":1}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2,"requestId":1}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requeſtId":1}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"reque\u017ftId":1}}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	if r := l.next(t, 2*time.Second); replyID(r) != 3 {
		t.Fatalf("want the ping reply, got %v", r)
	}
	time.Sleep(50 * time.Millisecond)
	if b, _ := os.ReadFile(h.logPath); strings.Contains(string(b), "result=cancelled") {
		t.Fatalf("a cancel with a bad key stopped the wait:\n%s", b)
	}
	l.send(t, cancelNote(1))
	waitForLog(t, h.logPath, "result=cancelled", 2*time.Second)
	l.close(t, 2*time.Second)
}

func TestEOFCancelsAndWaitsForCalls(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever), call(2, "tmux_wait", waitForever))
	waitUntil(t, "both waits poll", func() bool { return h.f.count("capture-pane") >= 2 })
	l.close(t, 2*time.Second)
	// Serve has returned, so both calls must already have finished: audited and answered.
	b, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "result=cancelled"); n != 2 {
		t.Errorf("want 2 cancelled lines when Serve returns, got %d:\n%s", n, b)
	}
	rs := l.rest()
	if len(rs) != 2 {
		t.Fatalf("want a reply for each wait, got %v", rs)
	}
	for _, r := range rs {
		wantCode(t, "wait at EOF", textOf(t, r), true, "cancelled")
	}
}

func textOf(t *testing.T, r map[string]any) string {
	t.Helper()
	text, _ := toolText(t, r)
	return text
}

func TestConcurrentRepliesDoNotInterleave(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever))
	var lines []string
	for id := 2; id < maxInFlight; id++ {
		tool := "tmux_list"
		if id%2 == 0 {
			tool = "tmux_whoami"
		}
		lines = append(lines, call(id, tool, ""))
	}
	l.send(t, lines...)
	seen := map[int]int{}
	for range lines {
		r := l.next(t, 5*time.Second)
		if _, isErr := toolText(t, r); isErr {
			t.Errorf("reply %v is an error", r)
		}
		seen[replyID(r)]++
	}
	for id := 2; id < maxInFlight; id++ {
		if seen[id] != 1 {
			t.Errorf("id %d answered %d times", id, seen[id])
		}
	}
	l.send(t, cancelNote(1))
	l.close(t, 2*time.Second)
}

func TestDuplicateInFlightID(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	l.send(t, call(1, "tmux_wait", waitForever), call(1, "tmux_list", ""))
	// The rejection carries id null: with id 1 the client would take it as the answer to the wait.
	if r := l.next(t, 2*time.Second); r["id"] != nil || errCode(r) != codeInvalidRequest {
		t.Fatalf("a second request with an in-flight id must be rejected with id null, got %v", r)
	}
	l.send(t, cancelNote(1))
	l.close(t, 2*time.Second)
}

func TestInFlightCap(t *testing.T) {
	h := newLayoutHarness(t)
	l := serveLive(t, h.s)
	for id := 1; id <= maxInFlight; id++ {
		l.send(t, call(id, "tmux_wait", waitForever))
	}
	l.send(t, call(maxInFlight+1, "tmux_list", ""))
	r := l.next(t, 2*time.Second)
	if replyID(r) != maxInFlight+1 {
		t.Fatalf("want the reply to the call over the cap, got %v", r)
	}
	text, isErr := toolText(t, r)
	wantCode(t, "call over the cap", text, isErr, "refused")
	waitForLog(t, h.logPath, "tool=tmux_list target=- result=refused", time.Second)
	l.close(t, 2*time.Second)
}

func TestMain(m *testing.M) {
	if os.Getenv("AGENT_TMUX_TEST_CHILD") == "sigpipe" {
		os.Exit(sigpipeChild(os.Getenv("AGENT_TMUX_TEST_DIR")))
	}
	os.Exit(m.Run())
}

// gateRunner holds every new-window until the file release exists, so a call can be kept between its intent
// line and its outcome.
type gateRunner struct {
	*fakeTmux
	release string
}

func (g *gateRunner) Run(args []string) (string, error) {
	if args[0] == "new-window" {
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(g.release); err == nil {
				break
			}
		}
	}
	return g.fakeTmux.Run(args)
}

// sigpipeChild is the server TestSIGPIPEDoesNotKill runs in a separate process, on its real stdin and stdout.
func sigpipeChild(dir string) int {
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		return 2
	}
	f := standardFake()
	f.panes[0]["pane_current_path"] = src
	s := NewServer(&gateRunner{f, filepath.Join(dir, "release")}, Home{PaneID: "%1", WindowID: "@1", SessionID: "$1"},
		filepath.Join(dir, "agent-tmux.log"), log.New(os.Stderr, "", 0))
	s.userHome = dir
	if err := serveStdio(s); err != nil {
		return 1
	}
	return 0
}

// lockedBuffer collects a child's stderr while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A reply written after the client closed stdout must not kill the server while another call sits between its
// intent line and its outcome.
func TestSIGPIPEDoesNotKill(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "agent-tmux.log")
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "AGENT_TMUX_TEST_CHILD=sigpipe", "AGENT_TMUX_TEST_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = outW
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	outW.Close()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			<-exited
		}
	}()
	send := func(line string) {
		if _, err := io.WriteString(stdin, line+"\n"); err != nil {
			t.Fatalf("write to the server: %v", err)
		}
	}
	send(call(1, "tmux_new_window", `{}`))
	waitForLog(t, logPath, "tool=tmux_new_window target=$1 result=intent", 10*time.Second)
	outR.Close()
	send(call(2, "tmux_whoami", ""))
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(stderr.String(), "write reply"); {
		select {
		case err := <-exited:
			t.Fatalf("the server died writing a reply to a closed stdout: %v\nstderr:\n%s", err, stderr)
		case <-time.After(5 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("no failed reply write was logged; stderr:\n%s", stderr)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "release"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, logPath, "tool=tmux_new_window target=$1 result=ok", 10*time.Second)
	stdin.Close()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("server exit: %v\nstderr:\n%s", err, stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not exit at EOF")
	}
	b, _ := os.ReadFile(logPath)
	var intent, outcome string
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) > 3 && strings.Contains(l, "tool=tmux_new_window") {
			if strings.Contains(l, "result=intent") {
				intent = f[1] + " " + f[2]
			} else if strings.Contains(l, "result=ok") {
				outcome = f[1] + " " + f[2]
			}
		}
	}
	if intent == "" || intent != outcome {
		t.Errorf("intent %q and outcome %q must carry the same pid and call:\n%s", intent, outcome, b)
	}
}

// blockRunner holds every command named cmd until release is closed, and closes entered at the first one.
type blockRunner struct {
	*fakeTmux
	cmd              string
	entered, release chan struct{}
	once             sync.Once
}

func (b *blockRunner) Run(args []string) (string, error) {
	if args[0] == b.cmd {
		b.once.Do(func() { close(b.entered) })
		<-b.release
	}
	return b.fakeTmux.Run(args)
}

// failWriter fails every write, as stdout does once the client has gone.
type failWriter struct{ n atomic.Int32 }

func (w *failWriter) Write([]byte) (int, error) {
	w.n.Add(1)
	return 0, errors.New("write |1: broken pipe")
}

// A reply the read loop cannot write ends the loop with stdin still open, and Serve still waits for the call
// that is running before it returns.
func TestReplyWriteFailureEndsLoop(t *testing.T) {
	h := newLayoutHarness(t)
	b := &blockRunner{fakeTmux: h.f, cmd: "capture-pane", entered: make(chan struct{}), release: make(chan struct{})}
	h.s.tm = tmux{b}
	pr, pw := io.Pipe()
	defer pw.Close()
	w := &failWriter{}
	done := make(chan error, 1)
	go func() { done <- h.s.Serve(pr, w) }()
	go io.WriteString(pw, call(1, "tmux_capture", `{"pane":"%2"}`)+"\n")
	select {
	case <-b.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the capture never reached tmux")
	}
	go io.WriteString(pw, `{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n")
	waitUntil(t, "the ping reply is written", func() bool { return w.n.Load() == 1 })
	select {
	case err := <-done:
		close(b.release)
		t.Fatalf("Serve returned while the capture was running: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(b.release)
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve returned no error after a failed reply")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve kept reading after a reply it could not write")
	}
	if b, _ := os.ReadFile(h.logPath); !strings.Contains(string(b), "tool=tmux_capture target=%2 result=ok") {
		t.Errorf("the running capture did not finish before Serve returned:\n%s", b)
	}
	if n := w.n.Load(); n != 2 {
		t.Errorf("want the ping reply and the capture reply attempted, got %d writes", n)
	}
}
