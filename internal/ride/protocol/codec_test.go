package protocol

import (
	"encoding/json"
	"testing"
)

func TestEncodeCommand_EncodesCommandArray(t *testing.T) {
	codec := NewCodec()

	payload, err := codec.EncodeCommand("Execute", map[string]any{"text": "      1+1\n", "trace": 0})
	if err != nil {
		t.Fatalf("EncodeCommand failed: %v", err)
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil {
		t.Fatalf("payload is not valid JSON array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr))
	}

	var cmd string
	if err := json.Unmarshal(arr[0], &cmd); err != nil {
		t.Fatalf("failed to decode command: %v", err)
	}
	if cmd != "Execute" {
		t.Fatalf("unexpected command: %q", cmd)
	}

	var args map[string]any
	if err := json.Unmarshal(arr[1], &args); err != nil {
		t.Fatalf("failed to decode args: %v", err)
	}
	if args["text"] != "      1+1\n" {
		t.Fatalf("text arg mismatch: %v", args["text"])
	}
}

func TestDecodePayload_KnownCommand(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodePayload(`["Execute",{"text":"      1+1\n","trace":0}]`)
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	if decoded.Kind != KindCommand {
		t.Fatalf("expected KindCommand, got %v", decoded.Kind)
	}
	if !decoded.Known {
		t.Fatal("expected Execute to be marked as known command")
	}
	if decoded.Command != "Execute" {
		t.Fatalf("unexpected command: %q", decoded.Command)
	}
}

func TestDecodePayload_UnknownCommandIsTolerated(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodePayload(`["NewFutureCommand",{"x":1}]`)
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	if decoded.Kind != KindCommand {
		t.Fatalf("expected KindCommand, got %v", decoded.Kind)
	}
	if decoded.Known {
		t.Fatal("expected unknown command to be marked Known=false")
	}
	if decoded.Command != "NewFutureCommand" {
		t.Fatalf("unexpected command: %q", decoded.Command)
	}
}

func TestDecodePayload_NonJSONIsRaw(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodePayload("SupportedProtocols=2")
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	if decoded.Kind != KindRaw {
		t.Fatalf("expected KindRaw, got %v", decoded.Kind)
	}
	if decoded.Raw != "SupportedProtocols=2" {
		t.Fatalf("unexpected raw payload: %q", decoded.Raw)
	}
}

func TestDecodePayload_BadJSONFallsBackToRaw(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodePayload(`["Execute",{bad-json}]`)
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	if decoded.Kind != KindRaw {
		t.Fatalf("expected KindRaw, got %v", decoded.Kind)
	}
	if decoded.Raw == "" {
		t.Fatal("expected raw payload to be preserved")
	}
}

func TestNormalizeBool_SupportsBoolAndNumericFlags(t *testing.T) {
	cases := []struct {
		name     string
		input    any
		expected bool
		ok       bool
	}{
		{name: "bool true", input: true, expected: true, ok: true},
		{name: "bool false", input: false, expected: false, ok: true},
		{name: "float 1", input: float64(1), expected: true, ok: true},
		{name: "float 0", input: float64(0), expected: false, ok: true},
		{name: "int 1", input: 1, expected: true, ok: true},
		{name: "int 0", input: 0, expected: false, ok: true},
		{name: "unsupported", input: "1", expected: false, ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeBool(tc.input)
			if ok != tc.ok {
				t.Fatalf("ok mismatch: got %v want %v", ok, tc.ok)
			}
			if got != tc.expected {
				t.Fatalf("value mismatch: got %v want %v", got, tc.expected)
			}
		})
	}
}
