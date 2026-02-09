package adapter

import (
	"fmt"
	"sync"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
)

// Request is a minimal DAP request envelope used by the bootstrap server skeleton.
type Request struct {
	Seq       int
	Command   string
	Arguments any
}

// Response is a minimal DAP response envelope.
type Response struct {
	RequestSeq int
	Command    string
	Success    bool
	Message    string
	Body       any
}

// Event is a minimal DAP event envelope.
type Event struct {
	Event string
	Body  any
}

// Thread represents one DAP thread.
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ThreadsResponseBody is returned by DAP threads requests.
type ThreadsResponseBody struct {
	Threads []Thread `json:"threads"`
}

// StackFrame represents one DAP stack frame.
type StackFrame struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// StackTraceResponseBody is returned by DAP stackTrace requests.
type StackTraceResponseBody struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames"`
}

// Capabilities describes the adapter's currently supported DAP feature set.
type Capabilities struct {
	SupportsConfigurationDoneRequest  bool `json:"supportsConfigurationDoneRequest"`
	SupportsTerminateRequest          bool `json:"supportsTerminateRequest"`
	SupportsRestartRequest            bool `json:"supportsRestartRequest"`
	SupportsStepBack                  bool `json:"supportsStepBack"`
	SupportsFunctionBreakpoints       bool `json:"supportsFunctionBreakpoints"`
	SupportsConditionalBreakpoints    bool `json:"supportsConditionalBreakpoints"`
	SupportsHitConditionalBreakpoints bool `json:"supportsHitConditionalBreakpoints"`
	SupportsSetVariable               bool `json:"supportsSetVariable"`
	SupportsExceptionInfoRequest      bool `json:"supportsExceptionInfoRequest"`
	SupportsEvaluateForHovers         bool `json:"supportsEvaluateForHovers"`
}

type serverState int

const (
	stateCreated serverState = iota
	stateInitialized
	stateAttachedOrLaunched
	stateTerminated
)

// Server is the DAP adapter entry point.
type Server struct {
	mu                 sync.Mutex
	state              serverState
	capabilities       Capabilities
	rideController     RideCommandSender
	activeTracerWindow int
	activeTracerSet    bool
	activeThreadID     int
	activeThreadSet    bool
	tracerWindows      map[int]tracerWindowState
	threadCache        map[int]Thread
	threadOrder        []int
	siDescriptions     map[int][]string
	tracerOrder        []int
	syntheticThreadIDs map[string]int
	nextSyntheticID    int
	pauseFallback      func() error
}

// RideCommandSender sends mapped control commands to RIDE.
type RideCommandSender interface {
	SendCommand(command string, args any) error
}

type tracerWindowState struct {
	threadID int
	name     string
	line     int
	column   int
}

// StoppedEventBody is emitted for DAP stopped events synthesized from RIDE lifecycle signals.
type StoppedEventBody struct {
	Reason            string `json:"reason"`
	ThreadID          int    `json:"threadId"`
	AllThreadsStopped bool   `json:"allThreadsStopped"`
	Description       string `json:"description,omitempty"`
}

// NewServer creates a DAP server instance.
func NewServer() *Server {
	return &Server{
		state: stateCreated,
		capabilities: Capabilities{
			SupportsConfigurationDoneRequest:  true,
			SupportsTerminateRequest:          true,
			SupportsRestartRequest:            false,
			SupportsStepBack:                  false,
			SupportsFunctionBreakpoints:       false,
			SupportsConditionalBreakpoints:    false,
			SupportsHitConditionalBreakpoints: false,
			SupportsSetVariable:               false,
			SupportsExceptionInfoRequest:      false,
			SupportsEvaluateForHovers:         false,
		},
		tracerWindows:      map[int]tracerWindowState{},
		threadCache:        map[int]Thread{},
		threadOrder:        nil,
		siDescriptions:     map[int][]string{},
		tracerOrder:        nil,
		syntheticThreadIDs: map[string]int{},
		nextSyntheticID:    1000000,
	}
}

// SetRideController injects a RIDE command sender used by DAP control requests.
func (s *Server) SetRideController(controller RideCommandSender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rideController = controller
}

// SetActiveTracerWindow sets the current tracer window id used for step/continue commands.
func (s *Server) SetActiveTracerWindow(win int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTracerWindow = win
	s.activeTracerSet = true
}

// SetPauseFallback registers an optional fallback hook when WeakInterrupt fails.
func (s *Server) SetPauseFallback(fallback func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pauseFallback = fallback
}

// HandleRequest processes initialize/launch/attach/lifecycle skeleton requests.
func (s *Server) HandleRequest(req Request) (Response, []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == stateTerminated {
		return s.failure(req, "session already terminated"), nil
	}

	switch req.Command {
	case "initialize":
		if s.state != stateCreated {
			return s.failure(req, "initialize already completed"), nil
		}

		s.state = stateInitialized
		return Response{
				RequestSeq: req.Seq,
				Command:    req.Command,
				Success:    true,
				Body:       s.capabilities,
			},
			[]Event{{Event: "initialized"}}

	case "launch", "attach":
		if s.state != stateInitialized {
			return s.failure(req, "launch/attach requires initialize"), nil
		}
		s.state = stateAttachedOrLaunched
		return s.success(req), nil

	case "configurationDone":
		if s.state != stateAttachedOrLaunched {
			return s.failure(req, "configurationDone requires launch or attach"), nil
		}
		return s.success(req), nil

	case "disconnect", "terminate":
		s.state = stateTerminated
		return s.success(req), nil

	case "continue", "next", "stepIn", "stepOut", "pause":
		return s.handleControlCommand(req), nil
	case "threads":
		return s.handleThreadsRequest(req), nil
	case "stackTrace":
		return s.handleStackTraceRequest(req), nil

	default:
		return s.failure(req, "unsupported command"), nil
	}
}

func (s *Server) handleControlCommand(req Request) Response {
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "control command requires launch or attach")
	}
	if s.rideController == nil {
		return s.failure(req, "no RIDE controller configured")
	}

	switch req.Command {
	case "continue":
		return s.sendWindowCommand(req, "Continue")
	case "next":
		return s.sendWindowCommand(req, "RunCurrentLine")
	case "stepIn":
		return s.sendWindowCommand(req, "StepInto")
	case "stepOut":
		return s.sendWindowCommand(req, "ContinueTrace")
	case "pause":
		if err := s.rideController.SendCommand("WeakInterrupt", map[string]any{}); err != nil {
			if s.pauseFallback == nil {
				return s.failure(req, "WeakInterrupt failed and no pause fallback configured")
			}
			if fallbackErr := s.pauseFallback(); fallbackErr != nil {
				return s.failure(req, "WeakInterrupt and pause fallback failed")
			}
		}
		return s.success(req)
	default:
		return s.failure(req, "unsupported command")
	}
}

func (s *Server) sendWindowCommand(req Request, rideCommand string) Response {
	if !s.activeTracerSet {
		return s.failure(req, "no active tracer window")
	}
	if err := s.rideController.SendCommand(rideCommand, map[string]any{
		"win": s.activeTracerWindow,
	}); err != nil {
		return s.failure(req, "failed to send mapped RIDE control command")
	}
	return s.success(req)
}

func (s *Server) handleThreadsRequest(req Request) Response {
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "threads requires launch or attach")
	}
	if s.rideController == nil {
		return s.failure(req, "no RIDE controller configured")
	}
	if err := s.rideController.SendCommand("GetThreads", map[string]any{}); err != nil {
		return s.failure(req, "failed to request threads from RIDE")
	}
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
		Body: ThreadsResponseBody{
			Threads: s.snapshotThreads(),
		},
	}
}

func (s *Server) handleStackTraceRequest(req Request) Response {
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "stackTrace requires launch or attach")
	}

	threadID := extractThreadIDArgument(req.Arguments)
	if threadID <= 0 {
		return s.failure(req, "stackTrace requires threadId")
	}

	frames := s.buildStackFramesForThread(threadID)
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
		Body: StackTraceResponseBody{
			StackFrames: frames,
			TotalFrames: len(frames),
		},
	}
}

// HandleRidePayload updates adapter runtime stop state from inbound RIDE events.
func (s *Server) HandleRidePayload(decoded protocol.DecodedPayload) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if decoded.Kind != protocol.KindCommand {
		return nil
	}

	switch decoded.Command {
	case "ReplyGetThreads":
		reply, ok := extractReplyGetThreads(decoded.Args)
		if !ok {
			return nil
		}
		s.updateThreadCache(reply)
		return nil
	case "ReplyGetSIStack":
		reply, ok := extractReplyGetSIStack(decoded.Args)
		if !ok {
			return nil
		}
		si := make([]string, 0, len(reply.Stack))
		for _, frame := range reply.Stack {
			si = append(si, frame.Description)
		}
		s.siDescriptions[reply.Tid] = si
		return nil

	case "OpenWindow":
		window, ok := extractWindowContent(decoded.Args)
		if !ok || !window.Debugger {
			return nil
		}

		windowState := tracerWindowState{}
		if prior, exists := s.tracerWindows[window.Token]; exists {
			windowState = prior
		}
		if window.Tid > 0 {
			windowState.threadID = window.Tid
		}
		if window.Name != "" {
			windowState.name = window.Name
		}
		windowState.line = window.CurrentRow
		windowState.column = window.CurrentColumn

		if _, exists := s.tracerWindows[window.Token]; !exists {
			s.tracerOrder = append(s.tracerOrder, window.Token)
		}
		s.tracerWindows[window.Token] = windowState
		s.activeTracerWindow = window.Token
		s.activeTracerSet = true
		if windowState.threadID > 0 {
			s.activeThreadID = windowState.threadID
			s.activeThreadSet = true
		}

		return []Event{{
			Event: "stopped",
			Body:  s.newStoppedEventBody("entry", ""),
		}}

	case "SetHighlightLine":
		highlight, ok := extractSetHighlightLine(decoded.Args)
		if !ok {
			return nil
		}
		if highlight.Win > 0 {
			s.activeTracerWindow = highlight.Win
			s.activeTracerSet = true
		}
		if state, ok := s.tracerWindows[highlight.Win]; ok {
			state.line = highlight.Line
			state.column = highlight.StartCol
			s.tracerWindows[highlight.Win] = state
		}

		if threadID := s.threadForWindow(s.activeTracerWindow); threadID > 0 {
			s.activeThreadID = threadID
			s.activeThreadSet = true
		}
		return []Event{{
			Event: "stopped",
			Body:  s.newStoppedEventBody("step", ""),
		}}

	case "SetThread":
		setThread, ok := extractSetThread(decoded.Args)
		if !ok || setThread.Tid <= 0 {
			return nil
		}
		s.activeThreadID = setThread.Tid
		s.activeThreadSet = true
		return nil

	case "HadError":
		hadError, _ := extractHadError(decoded.Args)
		return []Event{{
			Event: "stopped",
			Body:  s.newStoppedEventBody("exception", hadError.ErrorText),
		}}

	default:
		return nil
	}
}

func (s *Server) updateThreadCache(reply protocol.ReplyGetThreadsArgs) {
	nextCache := make(map[int]Thread, len(reply.Threads))
	nextOrder := make([]int, 0, len(reply.Threads))

	for _, threadInfo := range reply.Threads {
		id := threadInfo.Tid
		if id <= 0 {
			key := threadInfo.Description
			if key == "" {
				key = threadInfo.State + "|" + threadInfo.Flags + "|" + threadInfo.Treq
			}
			if key == "" {
				key = "unknown"
			}

			syntheticID, ok := s.syntheticThreadIDs[key]
			if !ok {
				syntheticID = s.nextSyntheticID
				s.nextSyntheticID++
				s.syntheticThreadIDs[key] = syntheticID
			}
			id = syntheticID
		}

		name := threadInfo.Description
		if name == "" {
			name = fmt.Sprintf("Thread %d", id)
		}

		nextCache[id] = Thread{
			ID:   id,
			Name: name,
		}
		nextOrder = append(nextOrder, id)
	}

	s.threadCache = nextCache
	s.threadOrder = nextOrder
}

func (s *Server) snapshotThreads() []Thread {
	threads := make([]Thread, 0, len(s.threadOrder))
	seen := map[int]struct{}{}

	for _, id := range s.threadOrder {
		thread, ok := s.threadCache[id]
		if !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		threads = append(threads, thread)
	}

	return threads
}

func (s *Server) buildStackFramesForThread(threadID int) []StackFrame {
	frames := make([]StackFrame, 0)

	for i := len(s.tracerOrder) - 1; i >= 0; i-- {
		token := s.tracerOrder[i]
		window, ok := s.tracerWindows[token]
		if !ok {
			continue
		}
		if window.threadID != threadID {
			continue
		}

		name := window.name
		if name == "" {
			name = fmt.Sprintf("Frame %d", token)
		}

		frames = append(frames, StackFrame{
			ID:     token,
			Name:   name,
			Line:   oneBased(window.line),
			Column: oneBased(window.column),
		})
	}

	if names, ok := s.siDescriptions[threadID]; ok {
		for i := 0; i < len(frames) && i < len(names); i++ {
			if names[i] == "" {
				continue
			}
			frames[i].Name = names[i]
		}
	}

	return frames
}

func (s *Server) newStoppedEventBody(reason, description string) StoppedEventBody {
	return StoppedEventBody{
		Reason:            reason,
		ThreadID:          s.currentThreadID(),
		AllThreadsStopped: false,
		Description:       description,
	}
}

func (s *Server) currentThreadID() int {
	if s.activeThreadSet {
		return s.activeThreadID
	}
	if s.activeTracerSet {
		return s.threadForWindow(s.activeTracerWindow)
	}
	return 0
}

func (s *Server) threadForWindow(win int) int {
	state, ok := s.tracerWindows[win]
	if !ok {
		return 0
	}
	return state.threadID
}

func extractWindowContent(args any) (protocol.WindowContentArgs, bool) {
	switch v := args.(type) {
	case protocol.WindowContentArgs:
		return v, true
	case map[string]any:
		return protocol.WindowContentArgs{
			Token:    intFromAny(v["token"]),
			Debugger: boolFromAny(v["debugger"]),
			Tid:      intFromAny(v["tid"]),
		}, true
	default:
		return protocol.WindowContentArgs{}, false
	}
}

func extractSetHighlightLine(args any) (protocol.SetHighlightLineArgs, bool) {
	switch v := args.(type) {
	case protocol.SetHighlightLineArgs:
		return v, true
	case map[string]any:
		return protocol.SetHighlightLineArgs{
			Win: intFromAny(v["win"]),
		}, true
	default:
		return protocol.SetHighlightLineArgs{}, false
	}
}

func extractSetThread(args any) (protocol.SetThreadArgs, bool) {
	switch v := args.(type) {
	case protocol.SetThreadArgs:
		return v, true
	case map[string]any:
		return protocol.SetThreadArgs{
			Tid: intFromAny(v["tid"]),
		}, true
	default:
		return protocol.SetThreadArgs{}, false
	}
}

func extractHadError(args any) (protocol.HadErrorArgs, bool) {
	switch v := args.(type) {
	case protocol.HadErrorArgs:
		return v, true
	case map[string]any:
		return protocol.HadErrorArgs{
			Error:     intFromAny(v["error"]),
			ErrorText: stringFromAny(v["error_text"]),
			DMX:       v["dmx"],
		}, true
	default:
		return protocol.HadErrorArgs{}, false
	}
}

func extractReplyGetThreads(args any) (protocol.ReplyGetThreadsArgs, bool) {
	switch v := args.(type) {
	case protocol.ReplyGetThreadsArgs:
		return v, true
	case map[string]any:
		rawThreads, ok := v["threads"].([]any)
		if !ok {
			return protocol.ReplyGetThreadsArgs{}, false
		}

		threads := make([]protocol.ThreadInfo, 0, len(rawThreads))
		for _, raw := range rawThreads {
			threadMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			threads = append(threads, protocol.ThreadInfo{
				Description: stringFromAny(threadMap["description"]),
				State:       stringFromAny(threadMap["state"]),
				Tid:         intFromAny(threadMap["tid"]),
				Flags:       stringFromAny(threadMap["flags"]),
				Treq:        stringFromAny(threadMap["Treq"]),
			})
		}

		return protocol.ReplyGetThreadsArgs{Threads: threads}, true
	default:
		return protocol.ReplyGetThreadsArgs{}, false
	}
}

func extractReplyGetSIStack(args any) (protocol.ReplyGetSIStackArgs, bool) {
	switch v := args.(type) {
	case protocol.ReplyGetSIStackArgs:
		return v, true
	case map[string]any:
		rawStack, ok := v["stack"].([]any)
		if !ok {
			return protocol.ReplyGetSIStackArgs{}, false
		}

		stack := make([]protocol.SIStackEntry, 0, len(rawStack))
		for _, raw := range rawStack {
			entryMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			stack = append(stack, protocol.SIStackEntry{
				Description: stringFromAny(entryMap["description"]),
			})
		}

		return protocol.ReplyGetSIStackArgs{
			Tid:   intFromAny(v["tid"]),
			Stack: stack,
		}, true
	default:
		return protocol.ReplyGetSIStackArgs{}, false
	}
}

func extractThreadIDArgument(args any) int {
	if args == nil {
		return 0
	}
	m, ok := args.(map[string]any)
	if !ok {
		return 0
	}
	return intFromAny(m["threadId"])
}

func oneBased(value int) int {
	if value < 0 {
		return 1
	}
	return value + 1
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func boolFromAny(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b == 1
	case int8:
		return b == 1
	case int16:
		return b == 1
	case int32:
		return b == 1
	case int64:
		return b == 1
	case uint:
		return b == 1
	case uint8:
		return b == 1
	case uint16:
		return b == 1
	case uint32:
		return b == 1
	case uint64:
		return b == 1
	case float32:
		return b == 1
	case float64:
		return b == 1
	default:
		return false
	}
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Server) success(req Request) Response {
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
	}
}

func (s *Server) failure(req Request, msg string) Response {
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    false,
		Message:    msg,
	}
}
