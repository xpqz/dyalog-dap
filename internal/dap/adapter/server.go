package adapter

import (
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
	pauseFallback      func() error
}

// RideCommandSender sends mapped control commands to RIDE.
type RideCommandSender interface {
	SendCommand(command string, args any) error
}

type tracerWindowState struct {
	threadID int
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
		tracerWindows: map[int]tracerWindowState{},
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

// HandleRidePayload updates adapter runtime stop state from inbound RIDE events.
func (s *Server) HandleRidePayload(decoded protocol.DecodedPayload) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if decoded.Kind != protocol.KindCommand {
		return nil
	}

	switch decoded.Command {
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
