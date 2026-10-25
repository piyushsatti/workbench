package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	protocolDefault = "2025-06-18"
	serverName      = "agent-tmux"
	serverVersion   = "2026.10.13"
	maxLine         = 1 << 20
	// maxInFlight bounds concurrent tool calls: each tmux_wait holds one for up to minutes while it polls tmux.
	maxInFlight     = 16
	defaultProcRoot = "/proc"
)

var supportedProtocols = map[string]bool{"2025-06-18": true, "2025-03-26": true, "2024-11-05": true}

// JSON-RPC 2.0 error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

// Server holds what every call reads. The fields below the blank line change while it runs, each behind its own
// lock or as an atomic; the rest never change once Serve starts, so concurrent calls share them.
type Server struct {
	tm    tmux
	home  Home
	audit *Audit
	// events is the append-only event log (events.go). Nil means no event log: calls still work and say so.
	events *EventLog
	scope  captureScope
	log    *log.Logger
	// userHome bounds every start directory; empty makes every tool that needs one refuse.
	userHome string
	procRoot string
	waitPoll time.Duration
	// sweepEvery is how often Serve looks for moved and closed panes. Zero means never: main turns it on.
	sweepEvery time.Duration

	calls    atomic.Uint64
	typedSeq atomic.Uint64
	// homeSess is the home pane's session now. verifyHome re-reads it on every call, because the human can move
	// the Claude window to another session.
	homeMu   sync.Mutex
	homeSess string
	// reg remembers the panes this agent owns, to notice moves and closes between calls.
	regMu sync.Mutex
	reg   map[string]*tracked
}

// NewServer wires a server. logPath is the audit log; tests pass a temp path. An empty path makes every tool refuse.
// It has no event log and no sweeping until the caller sets them, so a test never writes outside its own directory.
func NewServer(r Runner, home Home, logPath string, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Server{tm: tmux{r}, home: home, audit: NewAudit(logPath), scope: defaultCaptureScope, log: logger,
		procRoot: defaultProcRoot, waitPoll: defaultWaitPoll, homeSess: home.SessionID, reg: map[string]*tracked{}}
}

// homeSession is the session the home pane is in as of the last verifyHome.
func (s *Server) homeSession() string {
	s.homeMu.Lock()
	defer s.homeMu.Unlock()
	return s.homeSess
}

func (s *Server) setHomeSession(id string) {
	s.homeMu.Lock()
	defer s.homeMu.Unlock()
	s.homeSess = id
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

var errLineTooLong = errors.New("line too long")

// conn is one Serve run: the stdout lock and the tool calls still in flight, keyed by request id.
type conn struct {
	s   *Server
	ctx context.Context // cancelled when stdin ends

	wmu sync.Mutex
	out io.Writer

	mu       sync.Mutex
	inFlight map[string]*flight
	wg       sync.WaitGroup
}

type flight struct {
	cancel context.CancelFunc
	// dropped: the client cancelled the call, so no reply is sent for it.
	dropped bool
}

// Serve reads newline-delimited JSON-RPC from in and writes replies to out until EOF. Each tools/call runs in
// its own goroutine, so a long tmux_wait never stalls the read loop. At EOF every call still running is
// cancelled, and Serve returns only after all of them have finished.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	ctx, stop := context.WithCancel(context.Background())
	c := &conn{s: s, ctx: ctx, out: out, inFlight: map[string]*flight{}}
	defer func() {
		stop()
		c.wg.Wait()
	}()
	if s.sweepEvery > 0 {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			s.sweep()
			t := time.NewTicker(s.sweepEvery)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					s.sweep()
				}
			}
		}()
	}
	br := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := readLine(br, maxLine)
		var resp *response
		if errors.Is(err, errLineTooLong) {
			resp = errResp(nil, codeInvalidRequest, "message too large")
		} else if len(bytes.TrimSpace(line)) > 0 {
			resp = c.handle(bytes.TrimSpace(line))
		}
		if resp != nil {
			if werr := c.write(resp); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil && !errors.Is(err, errLineTooLong) {
			return err
		}
	}
}

// write sends one whole message in a single Write under the lock, so replies from concurrent calls never interleave.
func (c *conn) write(resp *response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		c.s.log.Printf("marshal reply: %v", err)
		b, _ = json.Marshal(errResp(resp.ID, -32603, "internal error"))
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.out.Write(append(b, '\n'))
	return err
}

// start runs a tool call in its own goroutine. It returns a reply only when the call cannot start.
func (c *conn) start(id json.RawMessage, name string, args json.RawMessage) *response {
	key := idKey(id)
	c.mu.Lock()
	if c.inFlight[key] != nil {
		c.mu.Unlock()
		// Not the live id: a reply carrying it would be taken as the answer to the call still running.
		return errResp(nil, codeInvalidRequest, "a request with this id is still in flight")
	}
	if len(c.inFlight) >= maxInFlight {
		c.mu.Unlock()
		return okResp(id, c.s.refuseBusy(name))
	}
	ctx, cancel := context.WithCancel(c.ctx)
	f := &flight{cancel: cancel}
	c.inFlight[key] = f
	c.wg.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.wg.Done()
		res := c.s.callTool(ctx, name, args)
		c.mu.Lock()
		delete(c.inFlight, key)
		dropped := f.dropped
		c.mu.Unlock()
		cancel()
		if dropped {
			return
		}
		if err := c.write(okResp(id, res)); err != nil {
			c.s.log.Printf("write reply: %v", err)
		}
	}()
	return nil
}

// cancelCall handles notifications/cancelled. MCP says the cancelled request gets no reply, so it is dropped.
func (c *conn) cancelCall(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if checkKeys(params, []string{"requestId", "reason"}, true) != nil || json.Unmarshal(params, &p) != nil || p.RequestID == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if f := c.inFlight[idKey(p.RequestID)]; f != nil {
		f.dropped = true
		f.cancel()
	}
}

func idKey(id json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, id) != nil {
		return string(id)
	}
	return b.String()
}

// readLine returns one line without its limit being exceeded; an oversized line is drained and reported.
func readLine(br *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			buf = append(buf, chunk...)
			if len(buf) > max {
				tooLong, buf = true, nil
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if tooLong {
			return nil, errLineTooLong
		}
		return buf, err
	}
}

func errResp(id json.RawMessage, code int, msg string) *response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{code, msg}}
}

func okResp(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func (c *conn) handle(line []byte) *response {
	s := c.s
	if !json.Valid(line) {
		s.log.Printf("parse error on %d-byte message", len(line))
		return errResp(nil, codeParse, "parse error")
	}
	if line[0] != '{' {
		return errResp(nil, codeInvalidRequest, "invalid request (batches are not supported)")
	}
	// The envelope and every params object get the same key check as tool arguments (checkKeys).
	if err := checkKeys(line, []string{"jsonrpc", "id", "method", "params", "result", "error"}, true); err != nil {
		return errResp(nil, codeInvalidRequest, "invalid request: "+err.Error())
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errResp(nil, codeInvalidRequest, "invalid request")
	}
	notification := req.ID == nil
	if !notification {
		switch req.ID[0] {
		case '"', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'n':
		default:
			return errResp(nil, codeInvalidRequest, "invalid id")
		}
	}
	if req.Method == "" {
		if req.Result != nil || req.Error != nil {
			return nil // a response from the client; we never send requests, so ignore
		}
		return errResp(req.ID, codeInvalidRequest, "invalid request")
	}
	if req.JSONRPC != "2.0" {
		if notification {
			return nil
		}
		return errResp(req.ID, codeInvalidRequest, "jsonrpc must be \"2.0\"")
	}
	if notification {
		if req.Method == "notifications/cancelled" {
			c.cancelCall(req.Params)
		}
		return nil // never reply to a notification
	}
	switch req.Method {
	case "initialize":
		if err := checkKeys(req.Params, []string{"protocolVersion", "capabilities", "clientInfo"}, true); err != nil {
			return errResp(req.ID, codeInvalidParams, "initialize params: "+err.Error())
		}
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := protocolDefault
		if supportedProtocols[p.ProtocolVersion] {
			v = p.ProtocolVersion
		}
		if s.home.Err != nil {
			s.log.Printf("initialize while fail-closed: %v", s.home.Err)
		}
		return okResp(req.ID, map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
		})
	case "ping":
		return okResp(req.ID, map[string]any{})
	case "tools/list":
		return okResp(req.ID, map[string]any{"tools": toolDefs})
	case "tools/call":
		var p struct {
			Name      *string         `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := checkKeys(req.Params, []string{"name", "arguments", "_meta"}, true); err != nil {
			return errResp(req.ID, codeInvalidParams, "tools/call params: "+err.Error())
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == nil {
			return errResp(req.ID, codeInvalidParams, "tools/call needs params.name")
		}
		if !isTool(*p.Name) {
			s.auditWrite(auditRec{call: s.calls.Add(1), home: s.home.PaneID, tool: "unknown", result: "no_tool", args: []string{hashArg("name", *p.Name)}})
			return errResp(req.ID, codeInvalidParams, "unknown tool")
		}
		return c.start(req.ID, *p.Name, p.Arguments)
	}
	return errResp(req.ID, codeNoMethod, "method not found")
}

func (s *Server) auditWrite(r auditRec) error {
	err := s.audit.Write(r)
	if err != nil {
		s.log.Printf("audit log write failed: %v", err)
	}
	return err
}

var auditUnavailable = toolResult{Content: []textContent{{"text", "audit_unavailable: audit log could not be written; call refused"}}, IsError: true}

// callTool runs one tool. Every call is audited; if the audit line cannot be written the result is withheld.
// Tools that change tmux also write an intent line first (see intent), so they refuse before acting.
// ctx is cancelled by notifications/cancelled or stdin EOF. tmux_wait watches it throughout; a tool that changes
// tmux checks it once, in intent; the read-only tools finish without looking.
func (s *Server) callTool(ctx context.Context, name string, args json.RawMessage) toolResult {
	rec := auditRec{call: s.calls.Add(1), home: s.home.PaneID, tool: name}
	text, err := "", s.verifyHome()
	if err == nil {
		err = checkArgKeys(name, args)
	}
	if err == nil {
		switch name {
		case "tmux_whoami":
			text, err = s.toolWhoami(args, &rec)
		case "tmux_list":
			text, err = s.toolList(ctx, args, &rec)
		case "tmux_capture":
			text, err = s.toolCapture(args, &rec)
		case "tmux_wait":
			text, err = s.toolWait(ctx, args, &rec)
		case "tmux_new_window":
			text, err = s.toolNewWindow(ctx, args, &rec)
		case "tmux_split":
			text, err = s.toolSplit(ctx, args, &rec)
		case "tmux_rename_window":
			text, err = s.toolRenameWindow(ctx, args, &rec)
		case "tmux_label":
			text, err = s.toolLabel(ctx, args, &rec)
		case "tmux_note":
			text, err = s.toolNote(ctx, args, &rec)
		case "tmux_resize":
			text, err = s.toolResize(ctx, args, &rec)
		case "tmux_kill":
			text, err = s.toolKill(ctx, args, &rec)
		case "tmux_send":
			text, err = s.toolSend(ctx, args, &rec)
		}
	} else if name == "tmux_capture" && checkArgKeys(name, args) == nil {
		var a struct {
			Pane *string `json:"pane"`
		}
		if json.Unmarshal(args, &a) == nil && a.Pane != nil {
			rec.args = append(rec.args, hashArg("pane", *a.Pane))
		}
	}
	if err != nil {
		te := asToolErr(err, "internal")
		rec.result, rec.bytes = te.code, 0
		// A refusal can quote tmux-held text, such as a pane's command, so it gets the marker treatment too.
		text = neutralise(te.Error())
	} else if rec.result == "" {
		rec.result = "ok"
	}
	if aerr := s.auditWrite(rec); aerr != nil {
		return auditUnavailable
	}
	return toolResult{Content: []textContent{{"text", text}}, IsError: err != nil}
}

// refuseBusy answers a call that would exceed maxInFlight, audited like any other refusal.
func (s *Server) refuseBusy(name string) toolResult {
	if s.auditWrite(auditRec{call: s.calls.Add(1), home: s.home.PaneID, tool: name, result: "refused"}) != nil {
		return auditUnavailable
	}
	msg := fmt.Sprintf("refused: %d tool calls are already running; retry when one finishes", maxInFlight)
	return toolResult{Content: []textContent{{"text", msg}}, IsError: true}
}
