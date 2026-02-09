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
