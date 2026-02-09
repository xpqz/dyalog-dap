package adapter

import "sync"

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
	pauseFallback      func() error
}

// RideCommandSender sends mapped control commands to RIDE.
type RideCommandSender interface {
	SendCommand(command string, args any) error
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
