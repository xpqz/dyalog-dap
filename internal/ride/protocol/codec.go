package protocol

import "encoding/json"

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
	Args    any
	Known   bool
}

// ExecuteArgs is the typed argument model for Execute.
type ExecuteArgs struct {
	Text  string `json:"text"`
	Trace int    `json:"trace"`
}

// IdentifyArgs is the typed argument model for Identify.
type IdentifyArgs struct {
	APIVersion int `json:"apiVersion"`
	Identity   int `json:"identity"`
}

// ConnectArgs is the typed argument model for Connect.
type ConnectArgs struct {
	RemoteID int `json:"remoteId"`
}

// WindowContentArgs models OpenWindow and UpdateWindow payloads.
type WindowContentArgs struct {
	Token         int      `json:"token"`
	Name          string   `json:"name"`
	Filename      string   `json:"filename"`
	Text          []string `json:"text"`
	Debugger      bool     `json:"debugger"`
	EntityType    int      `json:"entityType"`
	Offset        int      `json:"offset"`
	ReadOnly      bool     `json:"readOnly"`
	CurrentRow    int      `json:"currentRow"`
	CurrentColumn int      `json:"currentColumn"`
	Stop          []int    `json:"stop"`
	Monitor       []int    `json:"monitor"`
	Trace         []int    `json:"trace"`
	Tid           int      `json:"tid"`
}

// WindowArgs models commands that reference a window token/id via "win".
type WindowArgs struct {
	Win int `json:"win"`
}

// SetPromptTypeArgs models SetPromptType.
type SetPromptTypeArgs struct {
	Type int `json:"type"`
}

// AppendSessionOutputArgs models AppendSessionOutput.
type AppendSessionOutputArgs struct {
	Result string `json:"result"`
	Type   int    `json:"type"`
	Group  int    `json:"group"`
}

// SetLineAttributesArgs models SetLineAttributes.
type SetLineAttributesArgs struct {
	Win     int   `json:"win"`
	Stop    []int `json:"stop"`
	Monitor []int `json:"monitor"`
	Trace   []int `json:"trace"`
}

// SetHighlightLineArgs models SetHighlightLine.
type SetHighlightLineArgs struct {
	Win      int `json:"win"`
	Line     int `json:"line"`
	EndLine  int `json:"end_line"`
	StartCol int `json:"start_col"`
	EndCol   int `json:"end_col"`
}

// WindowTypeChangedArgs models WindowTypeChanged.
type WindowTypeChangedArgs struct {
	Win    int  `json:"win"`
	Tracer bool `json:"tracer"`
}

// SaveChangesArgs models SaveChanges.
type SaveChangesArgs struct {
	Win     int      `json:"win"`
	Text    []string `json:"text"`
	Stop    []int    `json:"stop"`
	Monitor []int    `json:"monitor"`
	Trace   []int    `json:"trace"`
}

// ReplySaveChangesArgs models ReplySaveChanges.
type ReplySaveChangesArgs struct {
	Win int `json:"win"`
	Err int `json:"err"`
}

// GetThreadsArgs models GetThreads.
type GetThreadsArgs struct{}

// ThreadInfo models one thread record in ReplyGetThreads.
type ThreadInfo struct {
	Description string `json:"description"`
	State       string `json:"state"`
	Tid         int    `json:"tid"`
	Flags       string `json:"flags"`
	Treq        string `json:"Treq"`
}

// ReplyGetThreadsArgs models ReplyGetThreads.
type ReplyGetThreadsArgs struct {
	Threads []ThreadInfo `json:"threads"`
}

// SetThreadArgs models SetThread.
type SetThreadArgs struct {
	Tid int `json:"tid"`
}

// GetSIStackArgs models GetSIStack.
type GetSIStackArgs struct{}

// SIStackEntry models one stack entry in ReplyGetSIStack.
type SIStackEntry struct {
	Description string `json:"description"`
}

// ReplyGetSIStackArgs models ReplyGetSIStack.
type ReplyGetSIStackArgs struct {
	Stack []SIStackEntry `json:"stack"`
	Tid   int            `json:"tid"`
}

// HadErrorArgs models HadError optional metadata fields.
type HadErrorArgs struct {
	Error     int    `json:"error"`
	ErrorText string `json:"error_text"`
	DMX       any    `json:"dmx"`
}

// DisconnectArgs models Disconnect.
type DisconnectArgs struct {
	Message string `json:"message"`
}

// SysErrorArgs models SysError.
type SysErrorArgs struct {
	Text  string `json:"text"`
	Stack string `json:"stack"`
}

// UnknownCommandArgs models UnknownCommand.
type UnknownCommandArgs struct {
	Name string `json:"name"`
}

// InternalErrorArgs models InternalError.
type InternalErrorArgs struct {
	Error     int    `json:"error"`
	ErrorText string `json:"error_text"`
	DMX       any    `json:"dmx"`
	Message   string `json:"message"`
}

// EmptyArgs represents commands with empty argument objects.
type EmptyArgs struct{}

// Codec handles encoding and decoding RIDE protocol messages.
type Codec struct {
	knownCommands map[string]struct{}
	decoders      map[string]func(args map[string]any) any
}

// NewCodec creates a message codec.
func NewCodec() *Codec {
	knownCommands := map[string]struct{}{
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
	}

	return &Codec{
		knownCommands: knownCommands,
		decoders: map[string]func(args map[string]any) any{
			"Identify":            decodeIdentifyArgs,
			"Connect":             decodeConnectArgs,
			"GetWindowLayout":     decodeEmptyArgs,
			"Execute":             decodeExecuteArgs,
			"SetPromptType":       decodeSetPromptTypeArgs,
			"AppendSessionOutput": decodeAppendSessionOutputArgs,
			"OpenWindow":          decodeWindowContentArgs,
			"UpdateWindow":        decodeWindowContentArgs,
			"CloseWindow":         decodeWindowArgs,
			"SetLineAttributes":   decodeSetLineAttributesArgs,
			"SetHighlightLine":    decodeSetHighlightLineArgs,
			"StepInto":            decodeWindowArgs,
			"RunCurrentLine":      decodeWindowArgs,
			"ContinueTrace":       decodeWindowArgs,
			"Continue":            decodeWindowArgs,
			"TraceBackward":       decodeWindowArgs,
			"TraceForward":        decodeWindowArgs,
			"RestartThreads":      decodeEmptyArgs,
			"WeakInterrupt":       decodeEmptyArgs,
			"StrongInterrupt":     decodeEmptyArgs,
			"GetThreads":          decodeGetThreadsArgs,
			"ReplyGetThreads":     decodeReplyGetThreadsArgs,
			"SetThread":           decodeSetThreadArgs,
			"GetSIStack":          decodeGetSIStackArgs,
			"ReplyGetSIStack":     decodeReplyGetSIStackArgs,
			"SaveChanges":         decodeSaveChangesArgs,
			"ReplySaveChanges":    decodeReplySaveChangesArgs,
			"HadError":            decodeHadErrorArgs,
			"Disconnect":          decodeDisconnectArgs,
			"SysError":            decodeSysErrorArgs,
			"UnknownCommand":      decodeUnknownCommandArgs,
			"InternalError":       decodeInternalErrorArgs,
			"WindowTypeChanged":   decodeWindowTypeChangedArgs,
		},
	}
}

// EncodeCommand marshals a command payload as ["Command",{...}].
func (c *Codec) EncodeCommand(command string, args any) (string, error) {
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
		return DecodedPayload{Kind: KindRaw, Raw: payload}, nil
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &arr); err != nil || len(arr) < 2 {
		return DecodedPayload{Kind: KindRaw, Raw: payload}, nil
	}

	var command string
	if err := json.Unmarshal(arr[0], &command); err != nil {
		return DecodedPayload{Kind: KindRaw, Raw: payload}, nil
	}

	argsMap := map[string]any{}
	if err := json.Unmarshal(arr[1], &argsMap); err != nil {
		return DecodedPayload{Kind: KindRaw, Raw: payload}, nil
	}

	_, known := c.knownCommands[command]
	decodedArgs := any(argsMap)
	if known {
		if decoder, ok := c.decoders[command]; ok {
			decodedArgs = decoder(argsMap)
		}
	}

	return DecodedPayload{
		Kind:    KindCommand,
		Command: command,
		Args:    decodedArgs,
		Known:   known,
	}, nil
}

func decodeEmptyArgs(map[string]any) any {
	return EmptyArgs{}
}

func decodeIdentifyArgs(args map[string]any) any {
	return IdentifyArgs{
		APIVersion: getInt(args, "apiVersion"),
		Identity:   getInt(args, "identity"),
	}
}

func decodeConnectArgs(args map[string]any) any {
	return ConnectArgs{RemoteID: getInt(args, "remoteId")}
}

func decodeExecuteArgs(args map[string]any) any {
	return ExecuteArgs{
		Text:  getString(args, "text"),
		Trace: getInt(args, "trace"),
	}
}

func decodeSetPromptTypeArgs(args map[string]any) any {
	return SetPromptTypeArgs{Type: getInt(args, "type")}
}

func decodeAppendSessionOutputArgs(args map[string]any) any {
	return AppendSessionOutputArgs{
		Result: getString(args, "result"),
		Type:   getInt(args, "type"),
		Group:  getInt(args, "group"),
	}
}

func decodeWindowContentArgs(args map[string]any) any {
	debugger, _ := NormalizeBool(args["debugger"])
	readOnly, _ := NormalizeBool(args["readOnly"])
	return WindowContentArgs{
		Token:         getInt(args, "token"),
		Name:          getString(args, "name"),
		Filename:      getString(args, "filename"),
		Text:          getStringSlice(args, "text"),
		Debugger:      debugger,
		EntityType:    getInt(args, "entityType"),
		Offset:        getInt(args, "offset"),
		ReadOnly:      readOnly,
		CurrentRow:    getInt(args, "currentRow"),
		CurrentColumn: getInt(args, "currentColumn"),
		Stop:          getIntSlice(args, "stop"),
		Monitor:       getIntSlice(args, "monitor"),
		Trace:         getIntSlice(args, "trace"),
		Tid:           getInt(args, "tid"),
	}
}

func decodeWindowArgs(args map[string]any) any {
	return WindowArgs{Win: getInt(args, "win")}
}

func decodeSetLineAttributesArgs(args map[string]any) any {
	return SetLineAttributesArgs{
		Win:     getInt(args, "win"),
		Stop:    getIntSlice(args, "stop"),
		Monitor: getIntSlice(args, "monitor"),
		Trace:   getIntSlice(args, "trace"),
	}
}

func decodeSetHighlightLineArgs(args map[string]any) any {
	return SetHighlightLineArgs{
		Win:      getInt(args, "win"),
		Line:     getInt(args, "line"),
		EndLine:  getInt(args, "end_line"),
		StartCol: getInt(args, "start_col"),
		EndCol:   getInt(args, "end_col"),
	}
}

func decodeWindowTypeChangedArgs(args map[string]any) any {
	tracer, _ := NormalizeBool(args["tracer"])
	return WindowTypeChangedArgs{
		Win:    getInt(args, "win"),
		Tracer: tracer,
	}
}

func decodeSaveChangesArgs(args map[string]any) any {
	return SaveChangesArgs{
		Win:     getInt(args, "win"),
		Text:    getStringSlice(args, "text"),
		Stop:    getIntSlice(args, "stop"),
		Monitor: getIntSlice(args, "monitor"),
		Trace:   getIntSlice(args, "trace"),
	}
}

func decodeReplySaveChangesArgs(args map[string]any) any {
	return ReplySaveChangesArgs{
		Win: getInt(args, "win"),
		Err: getInt(args, "err"),
	}
}

func decodeGetThreadsArgs(map[string]any) any {
	return GetThreadsArgs{}
}

func decodeReplyGetThreadsArgs(args map[string]any) any {
	threadsRaw := getSlice(args, "threads")
	threads := make([]ThreadInfo, 0, len(threadsRaw))
	for _, item := range threadsRaw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		threads = append(threads, ThreadInfo{
			Description: getString(m, "description"),
			State:       getString(m, "state"),
			Tid:         getInt(m, "tid"),
			Flags:       getString(m, "flags"),
			Treq:        getString(m, "Treq"),
		})
	}
	return ReplyGetThreadsArgs{Threads: threads}
}

func decodeSetThreadArgs(args map[string]any) any {
	return SetThreadArgs{Tid: getInt(args, "tid")}
}

func decodeGetSIStackArgs(map[string]any) any {
	return GetSIStackArgs{}
}

func decodeReplyGetSIStackArgs(args map[string]any) any {
	stackRaw := getSlice(args, "stack")
	stack := make([]SIStackEntry, 0, len(stackRaw))
	for _, item := range stackRaw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		stack = append(stack, SIStackEntry{Description: getString(m, "description")})
	}
	return ReplyGetSIStackArgs{
		Stack: stack,
		Tid:   getInt(args, "tid"),
	}
}

func decodeHadErrorArgs(args map[string]any) any {
	return HadErrorArgs{
		Error:     getInt(args, "error"),
		ErrorText: getString(args, "error_text"),
		DMX:       args["dmx"],
	}
}

func decodeDisconnectArgs(args map[string]any) any {
	return DisconnectArgs{Message: getString(args, "message")}
}

func decodeSysErrorArgs(args map[string]any) any {
	return SysErrorArgs{
		Text:  getString(args, "text"),
		Stack: getString(args, "stack"),
	}
}

func decodeUnknownCommandArgs(args map[string]any) any {
	return UnknownCommandArgs{Name: getString(args, "name")}
}

func decodeInternalErrorArgs(args map[string]any) any {
	return InternalErrorArgs{
		Error:     getInt(args, "error"),
		ErrorText: getString(args, "error_text"),
		DMX:       args["dmx"],
		Message:   getString(args, "message"),
	}
}

func getString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getInt(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	n, ok := toInt(v)
	if !ok {
		return 0
	}
	return n
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		return int(x), true
	case float32:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

func getStringSlice(args map[string]any, key string) []string {
	items := getSlice(args, key)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func getIntSlice(args map[string]any, key string) []int {
	items := getSlice(args, key)
	result := make([]int, 0, len(items))
	for _, item := range items {
		if n, ok := toInt(item); ok {
			result = append(result, n)
		}
	}
	return result
}

func getSlice(args map[string]any, key string) []any {
	v, ok := args[key]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	return items
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
