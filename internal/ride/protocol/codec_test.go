package protocol

import (
	"encoding/json"
	"testing"
)

func TestEncodeCommand_EncodesCommandArray(t *testing.T) {
	codec := NewCodec()

	payload, err := codec.EncodeCommand("Execute", ExecuteArgs{Text: "      1+1\n", Trace: 0})
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

	var args ExecuteArgs
	if err := json.Unmarshal(arr[1], &args); err != nil {
		t.Fatalf("failed to decode args: %v", err)
	}
	if args.Text != "      1+1\n" {
		t.Fatalf("text arg mismatch: %q", args.Text)
	}
	if args.Trace != 0 {
		t.Fatalf("trace arg mismatch: %d", args.Trace)
	}
}

func TestDecodePayload_KnownCommand_DecodesTypedArgs(t *testing.T) {
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

	executeArgs, ok := decoded.Args.(ExecuteArgs)
	if !ok {
		t.Fatalf("expected ExecuteArgs, got %T", decoded.Args)
	}
	if executeArgs.Text != "      1+1\n" {
		t.Fatalf("text mismatch: %q", executeArgs.Text)
	}
	if executeArgs.Trace != 0 {
		t.Fatalf("trace mismatch: %d", executeArgs.Trace)
	}
}

func TestDecodePayload_OpenWindow_DecodesTypedArgsAndNormalizesBoolFlags(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodePayload(`["OpenWindow",{
		"token":123,
		"name":"f",
		"text":["a←1","b←2"],
		"debugger":1,
		"readOnly":0,
		"entityType":1,
		"currentRow":2,
		"currentColumn":3,
		"stop":[1,2],
		"monitor":[3],
		"trace":[4]
	}]`)
	if err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}
	windowArgs, ok := decoded.Args.(WindowContentArgs)
	if !ok {
		t.Fatalf("expected WindowContentArgs, got %T", decoded.Args)
	}

	if windowArgs.Token != 123 {
		t.Fatalf("token mismatch: %d", windowArgs.Token)
	}
	if windowArgs.Name != "f" {
		t.Fatalf("name mismatch: %q", windowArgs.Name)
	}
	if !windowArgs.Debugger {
		t.Fatal("expected debugger=true")
	}
	if windowArgs.ReadOnly {
		t.Fatal("expected readOnly=false")
	}
	if len(windowArgs.Text) != 2 || windowArgs.Text[0] != "a←1" {
		t.Fatalf("text mismatch: %#v", windowArgs.Text)
	}
	if len(windowArgs.Stop) != 2 || windowArgs.Stop[0] != 1 || windowArgs.Stop[1] != 2 {
		t.Fatalf("stop mismatch: %#v", windowArgs.Stop)
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
	args, ok := decoded.Args.(map[string]any)
	if !ok {
		t.Fatalf("expected unknown args map, got %T", decoded.Args)
	}
	if args["x"].(float64) != 1 {
		t.Fatalf("unexpected unknown arg payload: %#v", args)
	}
}

func TestDecodePayload_UndocumentedCommandSubset_DecodesTypedArgs(t *testing.T) {
	codec := NewCodec()

	t.Run("GetWindowLayout", func(t *testing.T) {
		decoded, err := codec.DecodePayload(`["GetWindowLayout",{}]`)
		if err != nil {
			t.Fatalf("DecodePayload failed: %v", err)
		}
		if !decoded.Known {
			t.Fatal("expected GetWindowLayout to be known")
		}
		if _, ok := decoded.Args.(EmptyArgs); !ok {
			t.Fatalf("expected EmptyArgs, got %T", decoded.Args)
		}
	})

	t.Run("SetSIStack", func(t *testing.T) {
		decoded, err := codec.DecodePayload(`["SetSIStack",{"stack":"#.fn[2]"}]`)
		if err != nil {
			t.Fatalf("DecodePayload failed: %v", err)
		}
		if !decoded.Known {
			t.Fatal("expected SetSIStack to be known")
		}
		args, ok := decoded.Args.(SetSIStackArgs)
		if !ok {
			t.Fatalf("expected SetSIStackArgs, got %T", decoded.Args)
		}
		if args.Stack != "#.fn[2]" {
			t.Fatalf("stack mismatch: %q", args.Stack)
		}
	})

	t.Run("ExitMultilineInput", func(t *testing.T) {
		decoded, err := codec.DecodePayload(`["ExitMultilineInput",{}]`)
		if err != nil {
			t.Fatalf("DecodePayload failed: %v", err)
		}
		if !decoded.Known {
			t.Fatal("expected ExitMultilineInput to be known")
		}
		if _, ok := decoded.Args.(ExitMultilineInputArgs); !ok {
			t.Fatalf("expected ExitMultilineInputArgs, got %T", decoded.Args)
		}
	})

	t.Run("SetSessionLineGroup", func(t *testing.T) {
		decoded, err := codec.DecodePayload(`["SetSessionLineGroup",{"line_offset":3,"group":14}]`)
		if err != nil {
			t.Fatalf("DecodePayload failed: %v", err)
		}
		if !decoded.Known {
			t.Fatal("expected SetSessionLineGroup to be known")
		}
		args, ok := decoded.Args.(SetSessionLineGroupArgs)
		if !ok {
			t.Fatalf("expected SetSessionLineGroupArgs, got %T", decoded.Args)
		}
		if args.LineOffset != 3 || args.Group != 14 {
			t.Fatalf("unexpected decoded args: %#v", args)
		}
	})
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
