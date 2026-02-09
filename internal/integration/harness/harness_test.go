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
