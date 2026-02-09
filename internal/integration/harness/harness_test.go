package harness

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	dapadapter "github.com/stefan/lsp-dap/internal/dap/adapter"
	"github.com/stefan/lsp-dap/internal/ride/protocol"
	"github.com/stefan/lsp-dap/internal/ride/sessionstate"
)

func TestHarness_StartRequiresRideAddr(t *testing.T) {
	h := New(Config{TranscriptDir: t.TempDir()})
	_, err := h.Start(context.Background(), t.Name())
	if !errors.Is(err, ErrMissingRideAddr) {
		t.Fatalf("expected ErrMissingRideAddr, got %v", err)
	}
}

func TestHarness_StartInitializesSessionAndCapturesTranscript(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if err := writeFrame(conn, "SupportedProtocols=2"); err != nil {
			serverErr <- err
			return
		}
		first, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if first != "SupportedProtocols=2" {
			serverErr <- fmt.Errorf("unexpected first handshake frame: %q", first)
			return
		}
		second, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if second != "UsingProtocol=2" {
			serverErr <- fmt.Errorf("unexpected second handshake frame: %q", second)
			return
		}

		if err := writeFrame(conn, "UsingProtocol=2"); err != nil {
			serverErr <- err
			return
		}

		var commands []string
		for i := 0; i < 3; i++ {
			payload, err := readFrame(conn)
			if err != nil {
				serverErr <- err
				return
			}
			name, err := decodeCommandName(payload)
			if err != nil {
				serverErr <- err
				return
			}
			commands = append(commands, name)
		}

		expected := []string{"Identify", "Connect", "GetWindowLayout"}
		for i := range expected {
			if commands[i] != expected[i] {
				serverErr <- fmt.Errorf("command[%d] mismatch: got %q want %q", i, commands[i], expected[i])
				return
			}
		}

		serverErr <- nil
	}()

	h := New(Config{
		RideAddr:       ln.Addr().String(),
		ConnectTimeout: 2 * time.Second,
		TranscriptDir:  t.TempDir(),
	})
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	transcriptPath := h.TranscriptPath()
	if transcriptPath == "" {
		t.Fatal("expected transcript path")
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("fake server assertions failed: %v", err)
	}

	contents, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read transcript failed: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("expected non-empty transcript file")
	}
	if !strings.Contains(string(contents), "SupportedProtocols=2") {
		t.Fatalf("expected transcript to include handshake payloads, got: %s", string(contents))
	}
	if !strings.Contains(string(contents), "GetWindowLayout") {
		t.Fatalf("expected transcript to include GetWindowLayout command, got: %s", string(contents))
	}
}

func TestHarness_ExecutePromptTransitionLifecycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if err := runHandshake(conn); err != nil {
			serverErr <- err
			return
		}

		executePayload, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		command, err := decodeCommandName(executePayload)
		if err != nil {
			serverErr <- err
			return
		}
		if command != "Execute" {
			serverErr <- fmt.Errorf("expected Execute command, got %q", command)
			return
		}

		if err := writeFrame(conn, `["AppendSessionOutput",{"result":"      1+1\n","type":14,"group":0}]`); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["SetPromptType",{"type":0}]`); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["AppendSessionOutput",{"result":"2\n","type":3,"group":0}]`); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["SetPromptType",{"type":1}]`); err != nil {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	h := New(Config{
		RideAddr:       ln.Addr().String(),
		ConnectTimeout: 2 * time.Second,
		TranscriptDir:  t.TempDir(),
	})
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	events, _ := dispatcher.Subscribe(16)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	if err := dispatcher.SendCommand("Execute", protocol.ExecuteArgs{Text: "1+1\n", Trace: 0}); err != nil {
		t.Fatalf("SendCommand Execute failed: %v", err)
	}

	sawOutput := false
	sawBusy := false
	sawReady := false

	deadline := time.After(2 * time.Second)
	for !(sawOutput && sawBusy && sawReady) {
		select {
		case event := <-events:
			switch event.Command {
			case "AppendSessionOutput":
				sawOutput = true
			case "SetPromptType":
				args, ok := event.Args.(protocol.SetPromptTypeArgs)
				if !ok {
					t.Fatalf("expected SetPromptTypeArgs, got %T", event.Args)
				}
				if args.Type == 0 {
					sawBusy = true
				}
				if args.Type == 1 {
					sawReady = true
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for execute lifecycle events (output=%v busy=%v ready=%v)", sawOutput, sawBusy, sawReady)
		}
	}

	promptType, known := dispatcher.PromptType()
	if !known || promptType != 1 {
		t.Fatalf("expected promptType=1 after execute completion, got %d (known=%v)", promptType, known)
	}

	cancel()
	_ = h.Close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("fake server assertions failed: %v", err)
	}
}

func TestIntegration_TracerLifecycleAndSteppingControls(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if err := runHandshake(conn); err != nil {
			serverErr <- err
			return
		}

		if err := writeFrame(conn, `["OpenWindow",{"token":910,"debugger":1,"tid":7,"filename":"/ws/src/trace.apl"}]`); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["SetHighlightLine",{"win":910,"line":5,"end_line":5,"start_col":1,"end_col":1}]`); err != nil {
			serverErr <- err
			return
		}

		stepPayload, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		stepCommand, err := decodeCommandName(stepPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if stepCommand != "RunCurrentLine" {
			serverErr <- fmt.Errorf("expected RunCurrentLine, got %q", stepCommand)
			return
		}
		win, err := decodeCommandWin(stepPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if win != 910 {
			serverErr <- fmt.Errorf("expected step win=910, got %d", win)
			return
		}

		if err := writeFrame(conn, `["CloseWindow",{"win":910}]`); err != nil {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	h := New(Config{
		RideAddr:       ln.Addr().String(),
		ConnectTimeout: 2 * time.Second,
		TranscriptDir:  t.TempDir(),
	})
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer h.Close()

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	dispatcherEvents, _ := dispatcher.Subscribe(32)

	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	defer cancelDispatcher()
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatcherCtx)
		close(dispatcherDone)
	}()

	adapter := dapadapter.NewServer()
	adapter.SetRideController(dispatcher)

	if resp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 1, Command: "initialize"}); !resp.Success {
		t.Fatalf("adapter initialize failed: %s", resp.Message)
	}
	if resp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 2, Command: "launch"}); !resp.Success {
		t.Fatalf("adapter launch failed: %s", resp.Message)
	}

	highlightSeen := make(chan struct{})
	closeSeen := make(chan struct{})
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case event := <-dispatcherEvents:
				_ = adapter.HandleRidePayload(event)
				if event.Command == "SetHighlightLine" {
					select {
					case <-highlightSeen:
					default:
						close(highlightSeen)
					}
				}
				if event.Command == "CloseWindow" {
					select {
					case <-closeSeen:
					default:
						close(closeSeen)
					}
				}
			case <-dispatcherCtx.Done():
				return
			}
		}
	}()

	select {
	case <-highlightSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SetHighlightLine")
	}

	stepResp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 3, Command: "next"})
	if !stepResp.Success {
		t.Fatalf("expected next success, got %s", stepResp.Message)
	}

	select {
	case <-closeSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CloseWindow")
	}

	continueResp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 4, Command: "continue"})
	if continueResp.Success {
		t.Fatal("expected continue to fail after tracer window close")
	}

	cancelDispatcher()
	select {
	case <-dispatcherDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
	select {
	case <-bridgeDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("bridge did not stop")
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("fake server assertions failed: %v", err)
	}
}

func TestIntegration_SetBreakpointsSendsSetLineAttributes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if err := runHandshake(conn); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["OpenWindow",{"token":1001,"debugger":0,"tid":3,"filename":"/ws/src/bp.apl"}]`); err != nil {
			serverErr <- err
			return
		}

		payload, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		command, err := decodeCommandName(payload)
		if err != nil {
			serverErr <- err
			return
		}
		if command != "SetLineAttributes" {
			serverErr <- fmt.Errorf("expected SetLineAttributes, got %q", command)
			return
		}
		win, err := decodeCommandWin(payload)
		if err != nil {
			serverErr <- err
			return
		}
		if win != 1001 {
			serverErr <- fmt.Errorf("expected win=1001, got %d", win)
			return
		}
		stop, err := decodeCommandStop(payload)
		if err != nil {
			serverErr <- err
			return
		}
		if len(stop) != 2 || stop[0] != 2 || stop[1] != 7 {
			serverErr <- fmt.Errorf("expected stop=[2 7], got %#v", stop)
			return
		}

		serverErr <- nil
	}()

	h := New(Config{
		RideAddr:       ln.Addr().String(),
		ConnectTimeout: 2 * time.Second,
		TranscriptDir:  t.TempDir(),
	})
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer h.Close()

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	dispatcherEvents, _ := dispatcher.Subscribe(32)

	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	defer cancelDispatcher()
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatcherCtx)
		close(dispatcherDone)
	}()

	adapter := dapadapter.NewServer()
	adapter.SetRideController(dispatcher)
	if resp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 1, Command: "initialize"}); !resp.Success {
		t.Fatalf("adapter initialize failed: %s", resp.Message)
	}
	if resp, _ := adapter.HandleRequest(dapadapter.Request{Seq: 2, Command: "launch"}); !resp.Success {
		t.Fatalf("adapter launch failed: %s", resp.Message)
	}

	openSeen := make(chan struct{})
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case event := <-dispatcherEvents:
				_ = adapter.HandleRidePayload(event)
				if event.Command == "OpenWindow" {
					select {
					case <-openSeen:
					default:
						close(openSeen)
					}
				}
			case <-dispatcherCtx.Done():
				return
			}
		}
	}()

	select {
	case <-openSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OpenWindow mapping")
	}

	resp, _ := adapter.HandleRequest(dapadapter.Request{
		Seq:     3,
		Command: "setBreakpoints",
		Arguments: map[string]any{
			"source": map[string]any{
				"path": "/ws/src/bp.apl",
			},
			"breakpoints": []any{
				map[string]any{"line": 3},
				map[string]any{"line": 8},
			},
		},
	})
	if !resp.Success {
		t.Fatalf("setBreakpoints failed: %s", resp.Message)
	}

	cancelDispatcher()
	select {
	case <-dispatcherDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
	select {
	case <-bridgeDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("bridge did not stop")
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("fake server assertions failed: %v", err)
	}
}

func TestIntegration_SaveReplyCloseOrdering(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if err := runHandshake(conn); err != nil {
			serverErr <- err
			return
		}
		if err := writeFrame(conn, `["SetPromptType",{"type":0}]`); err != nil {
			serverErr <- err
			return
		}

		savePayload, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if command, _ := decodeCommandName(savePayload); command != "SaveChanges" {
			serverErr <- fmt.Errorf("expected SaveChanges first, got %q", command)
			return
		}

		if err := conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
			serverErr <- err
			return
		}
		if unexpectedPayload, err := readFrame(conn); err == nil {
			unexpectedCommand, _ := decodeCommandName(unexpectedPayload)
			serverErr <- fmt.Errorf("expected no command before ReplySaveChanges, got %q", unexpectedCommand)
			return
		} else {
			var netErr net.Error
			if !errors.As(err, &netErr) || !netErr.Timeout() {
				serverErr <- err
				return
			}
		}
		_ = conn.SetReadDeadline(time.Time{})

		if err := writeFrame(conn, `["ReplySaveChanges",{"win":5,"err":0}]`); err != nil {
			serverErr <- err
			return
		}

		closePayload, err := readFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		closeCommand, err := decodeCommandName(closePayload)
		if err != nil {
			serverErr <- err
			return
		}
		if closeCommand != "CloseWindow" {
			serverErr <- fmt.Errorf("expected CloseWindow after ReplySaveChanges, got %q", closeCommand)
			return
		}
		closeWin, err := decodeCommandWin(closePayload)
		if err != nil {
			serverErr <- err
			return
		}
		if closeWin != 5 {
			serverErr <- fmt.Errorf("expected CloseWindow win=5, got %d", closeWin)
			return
		}

		serverErr <- nil
	}()

	h := New(Config{
		RideAddr:       ln.Addr().String(),
		ConnectTimeout: 2 * time.Second,
		TranscriptDir:  t.TempDir(),
	})
	client, err := h.Start(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer h.Close()

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	dispatcherCtx, cancelDispatcher := context.WithCancel(context.Background())
	defer cancelDispatcher()
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatcherCtx)
		close(dispatcherDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		promptType, known := dispatcher.PromptType()
		if known && promptType == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for promptType=0")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := dispatcher.SendCommand("SaveChanges", protocol.SaveChangesArgs{
		Win:  5,
		Text: []string{"a←1"},
	}); err != nil {
		t.Fatalf("SendCommand SaveChanges failed: %v", err)
	}
	if err := dispatcher.SendCommand("CloseWindow", protocol.WindowArgs{Win: 5}); err != nil {
		t.Fatalf("SendCommand CloseWindow failed: %v", err)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("fake server assertions failed: %v", err)
	}

	cancelDispatcher()
	select {
	case <-dispatcherDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("dispatcher did not stop")
	}
}

func runHandshake(conn net.Conn) error {
	if err := writeFrame(conn, "SupportedProtocols=2"); err != nil {
		return err
	}
	first, err := readFrame(conn)
	if err != nil {
		return err
	}
	if first != "SupportedProtocols=2" {
		return fmt.Errorf("unexpected first handshake frame: %q", first)
	}
	second, err := readFrame(conn)
	if err != nil {
		return err
	}
	if second != "UsingProtocol=2" {
		return fmt.Errorf("unexpected second handshake frame: %q", second)
	}
	if err := writeFrame(conn, "UsingProtocol=2"); err != nil {
		return err
	}

	var commands []string
	for i := 0; i < 3; i++ {
		payload, err := readFrame(conn)
		if err != nil {
			return err
		}
		name, err := decodeCommandName(payload)
		if err != nil {
			return err
		}
		commands = append(commands, name)
	}

	expected := []string{"Identify", "Connect", "GetWindowLayout"}
	for i := range expected {
		if commands[i] != expected[i] {
			return fmt.Errorf("command[%d] mismatch: got %q want %q", i, commands[i], expected[i])
		}
	}

	return nil
}

func decodeCommandName(payload string) (string, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return "", err
	}
	if len(arr) < 1 {
		return "", errors.New("command payload missing array element")
	}
	var name string
	if err := json.Unmarshal(arr[0], &name); err != nil {
		return "", err
	}
	return name, nil
}

func decodeCommandWin(payload string) (int, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return 0, err
	}
	if len(arr) < 2 {
		return 0, errors.New("command payload missing args")
	}
	var args map[string]any
	if err := json.Unmarshal(arr[1], &args); err != nil {
		return 0, err
	}
	rawWin, ok := args["win"]
	if !ok {
		return 0, errors.New("command args missing win")
	}
	win, ok := rawWin.(float64)
	if !ok {
		return 0, errors.New("command win is not numeric")
	}
	return int(win), nil
}

func decodeCommandStop(payload string) ([]int, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return nil, err
	}
	if len(arr) < 2 {
		return nil, errors.New("command payload missing args")
	}
	var args map[string]any
	if err := json.Unmarshal(arr[1], &args); err != nil {
		return nil, err
	}
	rawStop, ok := args["stop"]
	if !ok {
		return nil, errors.New("command args missing stop")
	}
	items, ok := rawStop.([]any)
	if !ok {
		return nil, errors.New("stop is not an array")
	}
	stop := make([]int, 0, len(items))
	for _, item := range items {
		value, ok := item.(float64)
		if !ok {
			return nil, errors.New("stop contains non-numeric value")
		}
		stop = append(stop, int(value))
	}
	return stop, nil
}

func writeFrame(w io.Writer, payload string) error {
	frameLen := uint32(len(payload) + 8)
	if err := binary.Write(w, binary.BigEndian, frameLen); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "RIDE"); err != nil {
		return err
	}
	_, err := io.WriteString(w, payload)
	return err
}

func readFrame(r io.Reader) (string, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	if length < 8 {
		return "", fmt.Errorf("invalid frame length %d", length)
	}

	body := make([]byte, int(length)-4)
	if _, err := io.ReadFull(r, body); err != nil {
		return "", err
	}
	if string(body[:4]) != "RIDE" {
		return "", errors.New("invalid magic")
	}
	return string(body[4:]), nil
}
