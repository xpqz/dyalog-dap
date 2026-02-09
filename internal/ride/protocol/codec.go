package protocol

import (
	"encoding/json"
)

// PayloadKind indicates how a payload was interpreted.
type PayloadKind int

const (
	// KindRaw indicates a non-command payload (handshake strings or undecodable data).
	KindRaw PayloadKind = iota
	// KindCommand indicates a JSON command payload.
	KindCommand
)

// DecodedPayload is the normalized representation of inbound payload text.
type DecodedPayload struct {
	Kind    PayloadKind
	Raw     string
	Command string
	Args    map[string]any
	Known   bool
}

// Codec handles encoding and decoding RIDE protocol messages.
type Codec struct {
	knownCommands map[string]struct{}
}

// NewCodec creates a message codec.
func NewCodec() *Codec {
	return &Codec{
		knownCommands: map[string]struct{}{
			"Identify":              {},
			"Connect":               {},
			"GetWindowLayout":       {},
			"Execute":               {},
			"SetPromptType":         {},
			"AppendSessionOutput":   {},
			"OpenWindow":            {},
			"UpdateWindow":          {},
			"CloseWindow":           {},
			"SetLineAttributes":     {},
			"SetHighlightLine":      {},
			"StepInto":              {},
			"RunCurrentLine":        {},
			"ContinueTrace":         {},
			"Continue":              {},
			"TraceBackward":         {},
			"TraceForward":          {},
			"RestartThreads":        {},
			"WeakInterrupt":         {},
			"StrongInterrupt":       {},
			"GetThreads":            {},
			"ReplyGetThreads":       {},
			"SetThread":             {},
			"GetSIStack":            {},
			"ReplyGetSIStack":       {},
			"SaveChanges":           {},
			"ReplySaveChanges":      {},
			"HadError":              {},
			"Disconnect":            {},
			"SysError":              {},
			"UnknownCommand":        {},
			"InternalError":         {},
			"WindowTypeChanged":     {},
			"GetAutocomplete":       {},
			"ReplyGetAutocomplete":  {},
			"GetValueTip":           {},
			"ValueTip":              {},
			"GetConfiguration":      {},
			"ReplyGetConfiguration": {},
		},
	}
}

// EncodeCommand marshals a command payload as `["Command",{...}]`.
func (c *Codec) EncodeCommand(command string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal([]any{command, args})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// DecodePayload decodes payload text into command or raw representation.
// Non-JSON or malformed command payloads are returned as KindRaw.
func (c *Codec) DecodePayload(payload string) (DecodedPayload, error) {
	if len(payload) == 0 || payload[0] != '[' {
		return DecodedPayload{
			Kind: KindRaw,
			Raw:  payload,
		}, nil
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil || len(arr) < 2 {
		return DecodedPayload{
			Kind: KindRaw,
			Raw:  payload,
		}, nil
	}

	var command string
	if err := json.Unmarshal(arr[0], &command); err != nil {
		return DecodedPayload{
			Kind: KindRaw,
			Raw:  payload,
		}, nil
	}

	args := map[string]any{}
	if err := json.Unmarshal(arr[1], &args); err != nil {
		return DecodedPayload{
			Kind: KindRaw,
			Raw:  payload,
		}, nil
	}

	_, known := c.knownCommands[command]
	return DecodedPayload{
		Kind:    KindCommand,
		Command: command,
		Args:    args,
		Known:   known,
	}, nil
}

// NormalizeBool normalizes protocol bool-like values (bool or numeric 0/1).
func NormalizeBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case int8:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case int16:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case int32:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case int64:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case uint:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case uint8:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case uint16:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case uint32:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case uint64:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case float32:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	case float64:
		if x == 0 {
			return false, true
		}
		if x == 1 {
			return true, true
		}
	}
	return false, false
}
