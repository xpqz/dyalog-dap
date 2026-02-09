package transport

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestWritePayload_UsesRideFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient()
	client.AttachConn(clientConn)

	errCh := make(chan error, 1)
	go func() {
		payload, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if payload != "hello" {
			errCh <- fmt.Errorf("payload mismatch: got %q", payload)
			return
		}
		errCh <- nil
	}()

	if err := client.WritePayload("hello"); err != nil {
		t.Fatalf("WritePayload failed: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("frame assertion failed: %v", err)
	}
}

func TestReadPayload_RejectsBadMagic(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient()
	client.AttachConn(clientConn)

	errCh := make(chan error, 1)
	go func() {
		errCh <- writeCorruptFrame(serverConn, "oops")
	}()

	_, err := client.ReadPayload()
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
	if !errors.Is(err, ErrInvalidMagic) {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("writer failed: %v", werr)
	}
}

func TestInitializeSession_PerformsProtocol2HandshakeAndStartup(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient()
	client.AttachConn(clientConn)

	errCh := make(chan error, 1)
	go func() {
		if err := writeFrame(serverConn, "SupportedProtocols=2"); err != nil {
			errCh <- err
			return
		}

		p1, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if p1 != "SupportedProtocols=2" {
			errCh <- fmt.Errorf("unexpected first handshake response: %q", p1)
			return
		}

		p2, err := readFrame(serverConn)
		if err != nil {
			errCh <- err
			return
		}
		if p2 != "UsingProtocol=2" {
			errCh <- fmt.Errorf("unexpected second handshake response: %q", p2)
			return
		}

		if err := writeFrame(serverConn, "UsingProtocol=2"); err != nil {
			errCh <- err
			return
		}

		var commands []string
		for i := 0; i < 3; i++ {
			payload, err := readFrame(serverConn)
			if err != nil {
				errCh <- err
				return
			}
			name, err := decodeCommandName(payload)
			if err != nil {
				errCh <- err
				return
			}
			commands = append(commands, name)
		}

		expected := []string{"Identify", "Connect", "GetWindowLayout"}
		for i := range expected {
			if commands[i] != expected[i] {
				errCh <- fmt.Errorf("command[%d] mismatch: got %q want %q", i, commands[i], expected[i])
				return
			}
		}

		errCh <- nil
	}()

	if err := client.InitializeSession(); err != nil {
		t.Fatalf("InitializeSession failed: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server assertions failed: %v", err)
	}
}

func TestInitializeSession_ErrsOnUnexpectedServerProtocol(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient()
	client.AttachConn(clientConn)

	go func() {
		_ = writeFrame(serverConn, "SupportedProtocols=1")
	}()

	err := client.InitializeSession()
	if err == nil {
		t.Fatal("expected InitializeSession error")
	}
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

func writeCorruptFrame(w io.Writer, payload string) error {
	frameLen := uint32(len(payload) + 8)
	if err := binary.Write(w, binary.BigEndian, frameLen); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "NOPE"); err != nil {
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
		return "", fmt.Errorf("invalid length %d", length)
	}

	buf := make([]byte, int(length)-4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	if string(buf[:4]) != "RIDE" {
		return "", fmt.Errorf("invalid magic %q", string(buf[:4]))
	}
	return string(buf[4:]), nil
}

func decodeCommandName(payload string) (string, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		return "", err
	}
	if len(arr) < 1 {
		return "", fmt.Errorf("expected at least one element")
	}
	var command string
	if err := json.Unmarshal(arr[0], &command); err != nil {
		return "", err
	}
	return command, nil
}
