package transport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteAndReadPayloadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	message := map[string]any{
		"type": "request",
		"seq":  1,
	}

	if err := WritePayload(&buf, message); err != nil {
		t.Fatalf("WritePayload failed: %v", err)
	}

	payload, err := ReadPayload(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadPayload failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded["type"] != "request" {
		t.Fatalf("unexpected type: %#v", decoded["type"])
	}
}

func TestReadPayloadRequiresContentLength(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("X: 1\r\n\r\n{}"))
	_, err := ReadPayload(reader)
	if err == nil || !strings.Contains(err.Error(), "Content-Length") {
		t.Fatalf("expected Content-Length error, got %v", err)
	}
}
