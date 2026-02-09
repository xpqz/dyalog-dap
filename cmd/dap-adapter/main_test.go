package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestRun_InitializeAndDisconnectOverStdio(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	runErr := make(chan error, 1)
	go func() {
		runErr <- run(context.Background(), inR, outW, io.Discard)
		_ = outW.Close()
	}()

	decoderErr := make(chan error, 1)
	msgs := make(chan map[string]any, 32)
	go func() {
		defer close(msgs)
		decoderErr <- decodeDAPStream(outR, msgs)
	}()

	if err := writeDAPFrame(inW, map[string]any{
		"seq":     1,
		"type":    "request",
		"command": "initialize",
		"arguments": map[string]any{
			"adapterID": "dyalog-dap",
		},
	}); err != nil {
		t.Fatalf("write initialize failed: %v", err)
	}

	initResp := waitForResponse(t, msgs, 1)
	if ok, _ := initResp["success"].(bool); !ok {
		t.Fatalf("initialize failed response: %#v", initResp)
	}
	waitForEvent(t, msgs, "initialized")

	if err := writeDAPFrame(inW, map[string]any{
		"seq":     2,
		"type":    "request",
		"command": "disconnect",
	}); err != nil {
		t.Fatalf("write disconnect failed: %v", err)
	}
	_ = inW.Close()

	disconnectResp := waitForResponse(t, msgs, 2)
	if ok, _ := disconnectResp["success"].(bool); !ok {
		t.Fatalf("disconnect failed response: %#v", disconnectResp)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run to stop")
	}

	if err := <-decoderErr; err != nil {
		t.Fatalf("decode stream failed: %v", err)
	}
}

func TestRun_LaunchAndControlRoundTripAgainstRide(t *testing.T) {
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

		if err := rideHandshake(conn); err != nil {
			serverErr <- err
			return
		}

		// Establish active tracer state for step/stack requests.
		if err := rideWriteFrame(conn, `["OpenWindow",{"token":700,"debugger":1,"tid":7,"filename":"/ws/src/demo.apl","name":"demo","currentRow":4,"currentColumn":1}]`); err != nil {
			serverErr <- err
			return
		}
		if err := rideWriteFrame(conn, `["SetHighlightLine",{"win":700,"line":5,"end_line":5,"start_col":2,"end_col":2}]`); err != nil {
			serverErr <- err
			return
		}

		// First threads request polling trigger.
		threadsPayload, err := rideReadFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		threadsCommand, err := rideDecodeCommandName(threadsPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if threadsCommand != "GetThreads" {
			serverErr <- fmt.Errorf("expected GetThreads, got %q", threadsCommand)
			return
		}
		if err := rideWriteFrame(conn, `["ReplyGetThreads",{"threads":[{"tid":7,"description":"Main","state":"running","flags":"","Treq":""}]}]`); err != nil {
			serverErr <- err
			return
		}

		// Second threads request should happen as well.
		threadsPayload2, err := rideReadFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		threadsCommand2, err := rideDecodeCommandName(threadsPayload2)
		if err != nil {
			serverErr <- err
			return
		}
		if threadsCommand2 != "GetThreads" {
			serverErr <- fmt.Errorf("expected second GetThreads, got %q", threadsCommand2)
			return
		}
		if err := rideWriteFrame(conn, `["ReplyGetThreads",{"threads":[{"tid":7,"description":"Main","state":"running","flags":"","Treq":""}]}]`); err != nil {
			serverErr <- err
			return
		}

		nextPayload, err := rideReadFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		nextCommand, err := rideDecodeCommandName(nextPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if nextCommand != "RunCurrentLine" {
			serverErr <- fmt.Errorf("expected RunCurrentLine, got %q", nextCommand)
			return
		}

		breakpointPayload, err := rideReadFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		breakpointCommand, err := rideDecodeCommandName(breakpointPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if breakpointCommand != "SetLineAttributes" {
			serverErr <- fmt.Errorf("expected SetLineAttributes, got %q", breakpointCommand)
			return
		}
		win, stop, err := rideDecodeLineAttributes(breakpointPayload)
		if err != nil {
			serverErr <- err
			return
		}
		if win != 700 {
			serverErr <- fmt.Errorf("expected SetLineAttributes win=700, got %d", win)
			return
		}
		if len(stop) != 1 || stop[0] != 2 {
			serverErr <- fmt.Errorf("expected stop=[2], got %#v", stop)
			return
		}

		serverErr <- nil
	}()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	runErr := make(chan error, 1)
	go func() {
		runErr <- run(context.Background(), inR, outW, io.Discard)
		_ = outW.Close()
	}()

	decoderErr := make(chan error, 1)
	msgs := make(chan map[string]any, 128)
	go func() {
		defer close(msgs)
		decoderErr <- decodeDAPStream(outR, msgs)
	}()

	writeReq := func(seq int, command string, args map[string]any) {
		t.Helper()
		req := map[string]any{
			"seq":     seq,
			"type":    "request",
			"command": command,
		}
		if args != nil {
			req["arguments"] = args
		}
		if err := writeDAPFrame(inW, req); err != nil {
			t.Fatalf("write %s failed: %v", command, err)
		}
	}

	writeReq(1, "initialize", map[string]any{"adapterID": "dyalog-dap"})
	if ok, _ := waitForResponse(t, msgs, 1)["success"].(bool); !ok {
		t.Fatal("initialize response was not successful")
	}
	waitForEvent(t, msgs, "initialized")

	writeReq(2, "launch", map[string]any{
		"rideAddr":           ln.Addr().String(),
		"rideTranscriptsDir": t.TempDir(),
	})
	launchResp := waitForResponse(t, msgs, 2)
	if ok, _ := launchResp["success"].(bool); !ok {
		select {
		case err := <-serverErr:
			t.Fatalf("launch response was not successful: %#v (fake server err: %v)", launchResp, err)
		default:
			t.Fatalf("launch response was not successful: %#v", launchResp)
		}
	}
	waitForEvent(t, msgs, "stopped")

	// First threads request may return stale cache; second verifies refresh.
	writeReq(3, "threads", nil)
	if ok, _ := waitForResponse(t, msgs, 3)["success"].(bool); !ok {
		t.Fatal("first threads response was not successful")
	}
	writeReq(4, "threads", nil)
	threadsResp := waitForResponse(t, msgs, 4)
	if ok, _ := threadsResp["success"].(bool); !ok {
		t.Fatalf("second threads response failed: %#v", threadsResp)
	}

	writeReq(5, "stackTrace", map[string]any{"threadId": 7})
	stackResp := waitForResponse(t, msgs, 5)
	if ok, _ := stackResp["success"].(bool); !ok {
		t.Fatalf("stackTrace response failed: %#v", stackResp)
	}
	stackBody, _ := stackResp["body"].(map[string]any)
	frames, _ := stackBody["stackFrames"].([]any)
	if len(frames) == 0 {
		t.Fatalf("expected non-empty stackFrames: %#v", stackResp)
	}

	writeReq(6, "next", nil)
	if ok, _ := waitForResponse(t, msgs, 6)["success"].(bool); !ok {
		t.Fatal("next response was not successful")
	}

	writeReq(7, "setBreakpoints", map[string]any{
		"source": map[string]any{"path": "/ws/src/demo.apl"},
		"breakpoints": []any{
			map[string]any{"line": 3},
		},
	})
	breakpointsResp := waitForResponse(t, msgs, 7)
	if ok, _ := breakpointsResp["success"].(bool); !ok {
		t.Fatalf("setBreakpoints response failed: %#v", breakpointsResp)
	}

	writeReq(8, "disconnect", nil)
	if ok, _ := waitForResponse(t, msgs, 8)["success"].(bool); !ok {
		t.Fatal("disconnect response was not successful")
	}
	_ = inW.Close()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for run to stop")
	}
	if err := <-decoderErr; err != nil {
		t.Fatalf("decode stream failed: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("fake RIDE server assertions failed: %v", err)
	}
}

func TestRun_LaunchBeforeInitializeDoesNotStartRideRuntime(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	acceptErr := make(chan error, 1)
	go func() {
		_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(400 * time.Millisecond))
		conn, err := ln.Accept()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				acceptErr <- nil
				return
			}
			acceptErr <- err
			return
		}
		_ = conn.Close()
		acceptErr <- errors.New("unexpected RIDE connection accepted")
	}()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	runErr := make(chan error, 1)
	go func() {
		runErr <- run(context.Background(), inR, outW, io.Discard)
		_ = outW.Close()
	}()

	decoderErr := make(chan error, 1)
	msgs := make(chan map[string]any, 32)
	go func() {
		defer close(msgs)
		decoderErr <- decodeDAPStream(outR, msgs)
	}()

	if err := writeDAPFrame(inW, map[string]any{
		"seq":     1,
		"type":    "request",
		"command": "launch",
		"arguments": map[string]any{
			"rideAddr":           ln.Addr().String(),
			"rideTranscriptsDir": t.TempDir(),
		},
	}); err != nil {
		t.Fatalf("write launch failed: %v", err)
	}

	launchResp := waitForResponse(t, msgs, 1)
	if ok, _ := launchResp["success"].(bool); ok {
		t.Fatalf("expected launch before initialize to fail, got %#v", launchResp)
	}

	if err := writeDAPFrame(inW, map[string]any{
		"seq":     2,
		"type":    "request",
		"command": "disconnect",
	}); err != nil {
		t.Fatalf("write disconnect failed: %v", err)
	}
	_ = inW.Close()

	if ok, _ := waitForResponse(t, msgs, 2)["success"].(bool); !ok {
		t.Fatal("disconnect response was not successful")
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run to stop")
	}
	if err := <-decoderErr; err != nil {
		t.Fatalf("decode stream failed: %v", err)
	}
	if err := <-acceptErr; err != nil {
		t.Fatalf("launch-before-initialize side effect: %v", err)
	}
}

func waitForResponse(t *testing.T, msgs <-chan map[string]any, seq int) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				t.Fatalf("message stream closed while waiting for response %d", seq)
			}
			if msg["type"] != "response" {
				continue
			}
			requestSeq, _ := asInt(msg["request_seq"])
			if requestSeq != seq {
				continue
			}
			return msg
		case <-deadline:
			t.Fatalf("timed out waiting for response %d", seq)
		}
	}
}

func waitForEvent(t *testing.T, msgs <-chan map[string]any, event string) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				t.Fatalf("message stream closed while waiting for event %q", event)
			}
			if msg["type"] != "event" {
				continue
			}
			name, _ := msg["event"].(string)
			if name == event {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %q", event)
		}
	}
}

func writeDAPFrame(w io.Writer, message map[string]any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func decodeDAPStream(r io.Reader, out chan<- map[string]any) error {
	reader := bufio.NewReader(r)
	for {
		payload, err := readDAPPayload(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}
		out <- msg
	}
}

func rideHandshake(conn net.Conn) error {
	if err := rideWriteFrame(conn, "SupportedProtocols=2"); err != nil {
		return err
	}

	first, err := rideReadFrame(conn)
	if err != nil {
		return err
	}
	if first != "SupportedProtocols=2" {
		return fmt.Errorf("unexpected first handshake frame: %q", first)
	}
	second, err := rideReadFrame(conn)
	if err != nil {
		return err
	}
	if second != "UsingProtocol=2" {
		return fmt.Errorf("unexpected second handshake frame: %q", second)
	}
	if err := rideWriteFrame(conn, "UsingProtocol=2"); err != nil {
		return err
	}

	startup := []string{"Identify", "Connect", "GetWindowLayout"}
	for i, expected := range startup {
		payload, err := rideReadFrame(conn)
		if err != nil {
			return err
		}
		command, err := rideDecodeCommandName(payload)
		if err != nil {
			return err
		}
		if command != expected {
			return fmt.Errorf("startup command[%d] mismatch: got %q want %q", i, command, expected)
		}
	}
	return nil
}

func rideWriteFrame(w io.Writer, payload string) error {
	length := uint32(8 + len(payload))
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[:4], length)
	copy(header[4:], []byte("RIDE"))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := io.WriteString(w, payload)
	return err
}

func rideReadFrame(r io.Reader) (string, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return "", err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if string(header[4:]) != "RIDE" {
		return "", errors.New("invalid RIDE magic")
	}
	if length < 8 {
		return "", errors.New("invalid RIDE length")
	}
	payload := make([]byte, int(length)-8)
	if _, err := io.ReadFull(r, payload); err != nil {
		return "", err
	}
	return string(payload), nil
}

func rideDecodeCommandName(payload string) (string, error) {
	var envelope []any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return "", err
	}
	if len(envelope) < 1 {
		return "", errors.New("missing command name")
	}
	name, ok := envelope[0].(string)
	if !ok {
		return "", errors.New("command name is not a string")
	}
	return name, nil
}

func rideDecodeLineAttributes(payload string) (int, []int, error) {
	var envelope []any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return 0, nil, err
	}
	if len(envelope) < 2 {
		return 0, nil, errors.New("missing command args")
	}
	args, ok := envelope[1].(map[string]any)
	if !ok {
		return 0, nil, errors.New("command args are not an object")
	}
	win, _ := asInt(args["win"])
	stopAny, _ := args["stop"].([]any)
	stop := make([]int, 0, len(stopAny))
	for _, raw := range stopAny {
		value, ok := asInt(raw)
		if !ok {
			return 0, nil, errors.New("stop element is not numeric")
		}
		stop = append(stop, value)
	}
	return win, stop, nil
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}
