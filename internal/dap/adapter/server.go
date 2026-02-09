package adapter

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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

// Scope represents one DAP variable scope.
type Scope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	Expensive          bool   `json:"expensive"`
}

// ScopesResponseBody is returned by DAP scopes requests.
type ScopesResponseBody struct {
	Scopes []Scope `json:"scopes"`
}

// Variable represents one DAP variable entry.
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
}

// VariablesResponseBody is returned by DAP variables requests.
type VariablesResponseBody struct {
	Variables []Variable `json:"variables"`
}

// EvaluateResponseBody is returned by DAP evaluate requests.
type EvaluateResponseBody struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
}

// SourceResponseBody is returned by DAP source requests.
type SourceResponseBody struct {
	Content  string `json:"content"`
	MimeType string `json:"mimeType,omitempty"`
}

// Breakpoint describes one DAP breakpoint result.
type Breakpoint struct {
	Verified bool `json:"verified"`
	Line     int  `json:"line,omitempty"`
}

// SetBreakpointsResponseBody is returned by DAP setBreakpoints requests.
type SetBreakpointsResponseBody struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// PauseResponseBody reports which interrupt mechanism succeeded.
type PauseResponseBody struct {
	InterruptMethod string `json:"interruptMethod"`
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

const evaluateTimeout = 2 * time.Second
const localsFetchTimeout = 150 * time.Millisecond
const localsFetchPollInterval = 5 * time.Millisecond
const maxLocalValuePreviewRunes = 80
const maxLocalValueChildren = 32
const maxLocalSymbolsPerFrame = 64

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
	sourceByToken      map[int]sourceBinding
	tokenBySourceRef   map[int]int
	sourceRefByPath    map[string]int
	pathBySourceRef    map[int]string
	sourceTextByPath   map[string][]string
	pendingByPath      map[string][]int
	pendingBySourceRef map[int][]int
	nextSourceRef      int
	frameScopeRef      map[int]int
	variablesByRef     map[int][]Variable
	nextVariablesRef   int
	evaluateWaiters    map[int]chan evaluateResult
	nextEvaluateToken  int
	frameSymbols       map[int]frameSymbolsState
	pendingSymbolTips  map[int]pendingSymbolTip
	nextSymbolTipToken int
	promptType         int
	promptTypeSeen     bool
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

type sourceBinding struct {
	path        string
	sourceRef   int
	displayName string
}

type setBreakpointsArguments struct {
	path            string
	sourceReference int
	lines           []int
}

type evaluateArguments struct {
	expression string
	frameID    int
	context    string
}

type sourceArguments struct {
	path            string
	sourceReference int
}

type valueTipArgs struct {
	tip   []string
	class int
	token int
}

type evaluateResult struct {
	text  string
	class int
}

type frameSymbol struct {
	name     string
	isLocal  bool
	value    string
	class    int
	hasValue bool
}

type frameSymbolsState struct {
	order   []string
	symbols map[string]frameSymbol
}

type pendingSymbolTip struct {
	frameID int
	name    string
}

type symbolTipRequest struct {
	token int
	args  map[string]any
}

// StoppedEventBody is emitted for DAP stopped events synthesized from RIDE lifecycle signals.
type StoppedEventBody struct {
	Reason            string `json:"reason"`
	ThreadID          int    `json:"threadId"`
	AllThreadsStopped bool   `json:"allThreadsStopped"`
	Description       string `json:"description,omitempty"`
}

// OutputEventBody is emitted for DAP output events synthesized from RIDE diagnostics.
type OutputEventBody struct {
	Category string `json:"category,omitempty"`
	Output   string `json:"output"`
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
			SupportsEvaluateForHovers:         true,
		},
		tracerWindows:      map[int]tracerWindowState{},
		threadCache:        map[int]Thread{},
		threadOrder:        nil,
		siDescriptions:     map[int][]string{},
		tracerOrder:        nil,
		sourceByToken:      map[int]sourceBinding{},
		tokenBySourceRef:   map[int]int{},
		sourceRefByPath:    map[string]int{},
		pathBySourceRef:    map[int]string{},
		sourceTextByPath:   map[string][]string{},
		pendingByPath:      map[string][]int{},
		pendingBySourceRef: map[int][]int{},
		nextSourceRef:      1,
		frameScopeRef:      map[int]int{},
		variablesByRef:     map[int][]Variable{},
		nextVariablesRef:   1,
		evaluateWaiters:    map[int]chan evaluateResult{},
		nextEvaluateToken:  1,
		frameSymbols:       map[int]frameSymbolsState{},
		pendingSymbolTips:  map[int]pendingSymbolTip{},
		nextSymbolTipToken: 100000,
		syntheticThreadIDs: map[string]int{},
		nextSyntheticID:    1000000,
	}
}

// SetRideController injects a RIDE command sender used by DAP control requests.
func (s *Server) SetRideController(controller RideCommandSender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rideController = controller
	if controller == nil {
		s.evaluateWaiters = map[int]chan evaluateResult{}
		s.pendingSymbolTips = map[int]pendingSymbolTip{}
	}
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

// CanLaunchOrAttach reports whether launch/attach is currently valid.
func (s *Server) CanLaunchOrAttach() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == stateInitialized
}

// HandleRideReconnect resets transient runtime state and requests window layout rebuild.
func (s *Server) HandleRideReconnect() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == stateCreated {
		return nil
	}

	s.state = stateAttachedOrLaunched
	s.resetRuntimeStateForReconnect()
	s.requestWindowLayoutSync()

	return []Event{
		newOutputEvent("console", "RIDE reconnected; rebuilding window layout"),
	}
}

// ResolveSourceReferenceForToken returns the DAP source reference bound to a RIDE token.
func (s *Server) ResolveSourceReferenceForToken(token int) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.sourceByToken[token]
	if !ok {
		return 0, false
	}
	return binding.sourceRef, true
}

// ResolveTokenForSourceReference returns the active RIDE token bound to a DAP source reference.
func (s *Server) ResolveTokenForSourceReference(sourceRef int) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.tokenBySourceRef[sourceRef]
	return token, ok
}

// HandleRequest processes initialize/launch/attach/lifecycle skeleton requests.
func (s *Server) HandleRequest(req Request) (Response, []Event) {
	switch req.Command {
	case "evaluate":
		return s.handleEvaluateRequest(req), nil
	case "source":
		return s.handleSourceRequest(req), nil
	case "scopes":
		return s.handleScopesRequest(req), nil
	}

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
	case "variables":
		return s.handleVariablesRequest(req), nil
	case "setBreakpoints":
		return s.handleSetBreakpointsRequest(req), nil

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
			if strongErr := s.rideController.SendCommand("StrongInterrupt", map[string]any{}); strongErr == nil {
				return s.successWithBody(req, PauseResponseBody{InterruptMethod: "strong"})
			}
			if s.pauseFallback != nil {
				if fallbackErr := s.pauseFallback(); fallbackErr == nil {
					return s.successWithBody(req, PauseResponseBody{InterruptMethod: "fallback"})
				}
			}
			return s.failure(req, "WeakInterrupt and StrongInterrupt failed")
		}
		return s.successWithBody(req, PauseResponseBody{InterruptMethod: "weak"})
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

func (s *Server) handleSetBreakpointsRequest(req Request) Response {
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "setBreakpoints requires launch or attach")
	}
	if s.rideController == nil {
		return s.failure(req, "no RIDE controller configured")
	}

	args, ok := extractSetBreakpointsArguments(req.Arguments)
	if !ok {
		return s.failure(req, "setBreakpoints requires source and breakpoints")
	}

	token, mapped := s.resolveTokenForSetBreakpoints(args)
	if !mapped {
		s.deferBreakpoints(args)
		s.requestWindowLayoutSync()
		return Response{
			RequestSeq: req.Seq,
			Command:    req.Command,
			Success:    true,
			Body: SetBreakpointsResponseBody{
				Breakpoints: buildBreakpointResponses(args.lines, false),
			},
		}
	}

	stop := zeroBasedLines(args.lines)
	if err := s.rideController.SendCommand("SetLineAttributes", map[string]any{
		"win":     token,
		"stop":    stop,
		"monitor": []int{},
		"trace":   []int{},
	}); err != nil {
		return s.failure(req, "failed to send SetLineAttributes")
	}
	s.clearDeferredBreakpoints(args)

	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
		Body: SetBreakpointsResponseBody{
			Breakpoints: buildBreakpointResponses(args.lines, true),
		},
	}
}

func (s *Server) handleScopesRequest(req Request) Response {
	frameID := extractFrameIDArgument(req.Arguments)
	if frameID <= 0 {
		return s.failure(req, "scopes requires frameId")
	}

	s.mu.Lock()
	if s.state != stateAttachedOrLaunched {
		s.mu.Unlock()
		return s.failure(req, "scopes requires launch or attach")
	}
	if _, ok := s.tracerWindows[frameID]; !ok {
		s.mu.Unlock()
		return s.failure(req, "scopes requires valid frameId")
	}
	s.refreshFrameSymbolsLocked(frameID)
	controller := s.rideController
	requests := s.prepareSymbolTipRequestsLocked(frameID)
	s.mu.Unlock()

	for _, request := range requests {
		if controller == nil {
			break
		}
		if err := controller.SendCommand("GetValueTip", request.args); err != nil {
			s.mu.Lock()
			delete(s.pendingSymbolTips, request.token)
			s.mu.Unlock()
		}
	}
	if len(requests) > 0 {
		s.waitForSymbolTipRequests(frameID, localsFetchTimeout)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	scopeRef, ok := s.ensureScopeForFrame(frameID)
	if !ok {
		return s.failure(req, "scopes requires valid frameId")
	}

	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
		Body: ScopesResponseBody{
			Scopes: []Scope{{
			Name:               "Locals",
			VariablesReference: scopeRef,
			Expensive:          false,
		}},
		},
	}
}

func (s *Server) handleVariablesRequest(req Request) Response {
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "variables requires launch or attach")
	}

	varRef := extractVariablesReferenceArgument(req.Arguments)
	if varRef <= 0 {
		return s.failure(req, "variables requires variablesReference")
	}

	variables, ok := s.variablesByRef[varRef]
	if !ok {
		return s.failure(req, "unknown variablesReference")
	}

	return s.successWithBody(req, VariablesResponseBody{
		Variables: cloneVariables(variables),
	})
}

func (s *Server) handleEvaluateRequest(req Request) Response {
	args, ok := extractEvaluateArguments(req.Arguments)
	if !ok {
		return s.failure(req, "evaluate requires expression")
	}

	s.mu.Lock()
	if s.state == stateTerminated {
		s.mu.Unlock()
		return s.failure(req, "session already terminated")
	}
	if s.state != stateAttachedOrLaunched {
		s.mu.Unlock()
		return s.failure(req, "evaluate requires launch or attach")
	}
	if s.rideController == nil {
		s.mu.Unlock()
		return s.failure(req, "no RIDE controller configured")
	}
	if args.context != "watch" && args.context != "repl" && args.context != "hover" {
		s.mu.Unlock()
		return s.failure(req, "evaluate supports repl/watch contexts only")
	}
	if s.promptTypeSeen && s.promptType == 0 {
		s.mu.Unlock()
		return s.failure(req, "interpreter is busy; evaluate requires ready prompt")
	}

	win := args.frameID
	if win <= 0 && s.activeTracerSet {
		win = s.activeTracerWindow
	}
	if win <= 0 {
		s.mu.Unlock()
		return s.failure(req, "evaluate requires frameId or active tracer window")
	}

	token := s.nextEvaluateToken
	s.nextEvaluateToken++
	waiter := make(chan evaluateResult, 1)
	s.evaluateWaiters[token] = waiter
	controller := s.rideController
	s.mu.Unlock()

	if err := controller.SendCommand("GetValueTip", map[string]any{
		"win":       win,
		"line":      args.expression,
		"pos":       len([]rune(args.expression)),
		"maxWidth":  200,
		"maxHeight": 200,
		"token":     token,
	}); err != nil {
		s.mu.Lock()
		delete(s.evaluateWaiters, token)
		s.mu.Unlock()
		return s.failure(req, "failed to send GetValueTip")
	}

	select {
	case result := <-waiter:
		typeName := "string"
		if result.class != 0 {
			typeName = fmt.Sprintf("nameclass(%d)", result.class)
		}
		return s.successWithBody(req, EvaluateResponseBody{
			Result:             result.text,
			Type:               typeName,
			VariablesReference: 0,
		})
	case <-time.After(evaluateTimeout):
		s.mu.Lock()
		delete(s.evaluateWaiters, token)
		s.mu.Unlock()
		return s.failure(req, "timed out waiting for ValueTip")
	}
}

func (s *Server) handleSourceRequest(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == stateTerminated {
		return s.failure(req, "session already terminated")
	}
	if s.state != stateAttachedOrLaunched {
		return s.failure(req, "source requires launch or attach")
	}

	args, ok := extractSourceArguments(req.Arguments)
	if !ok {
		return s.failure(req, "source requires source.path or sourceReference")
	}

	path := args.path
	if args.sourceReference > 0 {
		if mapped, exists := s.pathBySourceRef[args.sourceReference]; exists {
			path = mapped
		}
	}
	if path == "" {
		return s.failure(req, "source requires a mapped sourceReference or path")
	}

	if lines, ok := s.sourceTextByPath[path]; ok {
		return s.successWithBody(req, SourceResponseBody{
			Content:  strings.Join(lines, "\n"),
			MimeType: "text/plain",
		})
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return s.failure(req, "source content is unavailable")
	}
	return s.successWithBody(req, SourceResponseBody{
		Content:  string(bytes),
		MimeType: "text/plain",
	})
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

	case "UpdateWindow":
		window, ok := extractWindowContent(decoded.Args)
		if !ok {
			return nil
		}
		s.bindTokenToSource(window.Token, window.Filename, window.Name)
		s.storeSourceText(window.Filename, window.Text)
		s.applyDeferredBreakpoints(window.Token)
		if window.Debugger {
			s.updateTracerWindow(window)
		}
		return nil

	case "CloseWindow":
		windowArgs, ok := extractWindowArgs(decoded.Args)
		if !ok {
			return nil
		}
		s.unbindToken(windowArgs.Win)
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
		if !ok {
			return nil
		}
		s.bindTokenToSource(window.Token, window.Filename, window.Name)
		s.storeSourceText(window.Filename, window.Text)
		s.applyDeferredBreakpoints(window.Token)
		if !window.Debugger {
			return nil
		}

		s.updateTracerWindow(window)
		s.activeTracerWindow = window.Token
		s.activeTracerSet = true
		windowState := s.tracerWindows[window.Token]
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

	case "SetPromptType":
		promptType, ok := extractSetPromptType(decoded.Args)
		if !ok {
			return nil
		}
		s.promptType = promptType
		s.promptTypeSeen = true
		return nil

	case "ValueTip":
		valueTip, ok := extractValueTip(decoded.Args)
		if !ok {
			return nil
		}
		s.dispatchValueTip(valueTip)
		return nil

	case "HadError":
		hadError, _ := extractHadError(decoded.Args)
		return []Event{{
			Event: "stopped",
			Body:  s.newStoppedEventBody("exception", hadError.ErrorText),
		}}
	case "Disconnect":
		disconnect, _ := extractDisconnect(decoded.Args)
		s.terminateSessionFromRide()
		return []Event{
			newOutputEvent("stderr", formatDisconnectMessage(disconnect)),
			{Event: "terminated", Body: map[string]any{}},
		}
	case "SysError":
		sysError, _ := extractSysError(decoded.Args)
		s.terminateSessionFromRide()
		return []Event{
			newOutputEvent("stderr", formatSysErrorMessage(sysError)),
			{Event: "terminated", Body: map[string]any{}},
		}
	case "InternalError":
		internalError, _ := extractInternalError(decoded.Args)
		s.terminateSessionFromRide()
		return []Event{
			newOutputEvent("stderr", formatInternalErrorMessage(internalError)),
			{Event: "terminated", Body: map[string]any{}},
		}
	case "UnknownCommand":
		unknown, _ := extractUnknownCommand(decoded.Args)
		return []Event{
			newOutputEvent("console", formatUnknownCommandMessage(unknown)),
		}

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

func (s *Server) bindTokenToSource(token int, path, displayName string) {
	if token <= 0 || path == "" {
		return
	}
	if existing, ok := s.sourceByToken[token]; ok {
		if mappedToken, ok := s.tokenBySourceRef[existing.sourceRef]; ok && mappedToken == token {
			delete(s.tokenBySourceRef, existing.sourceRef)
		}
	}

	sourceRef, ok := s.sourceRefByPath[path]
	if !ok {
		sourceRef = s.nextSourceRef
		s.nextSourceRef++
		s.sourceRefByPath[path] = sourceRef
		s.pathBySourceRef[sourceRef] = path
	}

	s.sourceByToken[token] = sourceBinding{
		path:        path,
		sourceRef:   sourceRef,
		displayName: displayName,
	}

	if existingToken, exists := s.tokenBySourceRef[sourceRef]; exists && existingToken != token {
		delete(s.sourceByToken, existingToken)
	}
	s.tokenBySourceRef[sourceRef] = token
}

func (s *Server) unbindToken(token int) {
	if token <= 0 {
		return
	}
	if binding, ok := s.sourceByToken[token]; ok {
		if mappedToken, mapped := s.tokenBySourceRef[binding.sourceRef]; mapped && mappedToken == token {
			delete(s.tokenBySourceRef, binding.sourceRef)
		}
		delete(s.sourceByToken, token)
	}

	if _, ok := s.tracerWindows[token]; ok {
		if scopeRef, exists := s.frameScopeRef[token]; exists {
			s.dropVariableReference(scopeRef)
			delete(s.frameScopeRef, token)
		}
		delete(s.frameSymbols, token)
		s.dropPendingSymbolTipsForFrame(token)
		delete(s.tracerWindows, token)
		s.removeTracerToken(token)
		if s.activeTracerSet && s.activeTracerWindow == token {
			s.activeTracerSet = false
		}
	}
}

func (s *Server) updateTracerWindow(window protocol.WindowContentArgs) {
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
}

func (s *Server) removeTracerToken(token int) {
	filtered := s.tracerOrder[:0]
	for _, current := range s.tracerOrder {
		if current == token {
			continue
		}
		filtered = append(filtered, current)
	}
	s.tracerOrder = filtered
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

func (s *Server) terminateSessionFromRide() {
	s.state = stateTerminated
	s.activeTracerSet = false
	s.activeThreadSet = false
	s.promptTypeSeen = false
	s.evaluateWaiters = map[int]chan evaluateResult{}
	s.pendingSymbolTips = map[int]pendingSymbolTip{}
}

func (s *Server) resetRuntimeStateForReconnect() {
	s.activeTracerSet = false
	s.activeThreadSet = false
	s.tracerWindows = map[int]tracerWindowState{}
	s.tracerOrder = nil
	s.threadCache = map[int]Thread{}
	s.threadOrder = nil
	s.siDescriptions = map[int][]string{}
	s.sourceByToken = map[int]sourceBinding{}
	s.tokenBySourceRef = map[int]int{}
	s.sourceTextByPath = map[string][]string{}
	s.frameScopeRef = map[int]int{}
	s.variablesByRef = map[int][]Variable{}
	s.nextVariablesRef = 1
	s.evaluateWaiters = map[int]chan evaluateResult{}
	s.frameSymbols = map[int]frameSymbolsState{}
	s.pendingSymbolTips = map[int]pendingSymbolTip{}
	s.nextSymbolTipToken = 100000
	s.promptTypeSeen = false
}

func newOutputEvent(category, output string) Event {
	return Event{
		Event: "output",
		Body: OutputEventBody{
			Category: category,
			Output:   ensureTrailingNewline(output),
		},
	}
}

func ensureTrailingNewline(value string) string {
	if value == "" {
		return "\n"
	}
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}

func formatDisconnectMessage(args protocol.DisconnectArgs) string {
	if args.Message == "" {
		return "RIDE disconnected"
	}
	return fmt.Sprintf("RIDE disconnected: %s", args.Message)
}

func formatSysErrorMessage(args protocol.SysErrorArgs) string {
	switch {
	case args.Text == "" && args.Stack == "":
		return "RIDE SysError"
	case args.Text == "":
		return fmt.Sprintf("RIDE SysError stack: %s", args.Stack)
	case args.Stack == "":
		return fmt.Sprintf("RIDE SysError: %s", args.Text)
	default:
		return fmt.Sprintf("RIDE SysError: %s\n%s", args.Text, args.Stack)
	}
}

func formatInternalErrorMessage(args protocol.InternalErrorArgs) string {
	parts := make([]string, 0, 3)
	if args.Message != "" {
		parts = append(parts, args.Message)
	}
	if args.ErrorText != "" {
		parts = append(parts, args.ErrorText)
	}
	if args.Error != 0 {
		parts = append(parts, fmt.Sprintf("code=%d", args.Error))
	}
	if len(parts) == 0 {
		return "RIDE InternalError"
	}
	return "RIDE InternalError: " + strings.Join(parts, " | ")
}

func formatUnknownCommandMessage(args protocol.UnknownCommandArgs) string {
	if args.Name == "" {
		return "RIDE reported unknown command"
	}
	return fmt.Sprintf("RIDE reported unknown command: %s", args.Name)
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
			Token:         intFromAny(v["token"]),
			Filename:      stringFromAny(v["filename"]),
			Name:          stringFromAny(v["name"]),
			Text:          stringSliceFromAny(v["text"]),
			Debugger:      boolFromAny(v["debugger"]),
			Tid:           intFromAny(v["tid"]),
			CurrentRow:    intFromAny(v["currentRow"]),
			CurrentColumn: intFromAny(v["currentColumn"]),
		}, true
	default:
		return protocol.WindowContentArgs{}, false
	}
}

func extractWindowArgs(args any) (protocol.WindowArgs, bool) {
	switch v := args.(type) {
	case protocol.WindowArgs:
		return v, true
	case map[string]any:
		return protocol.WindowArgs{
			Win: intFromAny(v["win"]),
		}, true
	default:
		return protocol.WindowArgs{}, false
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

func extractDisconnect(args any) (protocol.DisconnectArgs, bool) {
	switch v := args.(type) {
	case protocol.DisconnectArgs:
		return v, true
	case map[string]any:
		return protocol.DisconnectArgs{
			Message: stringFromAny(v["message"]),
		}, true
	default:
		return protocol.DisconnectArgs{}, false
	}
}

func extractSysError(args any) (protocol.SysErrorArgs, bool) {
	switch v := args.(type) {
	case protocol.SysErrorArgs:
		return v, true
	case map[string]any:
		return protocol.SysErrorArgs{
			Text:  stringFromAny(v["text"]),
			Stack: stringFromAny(v["stack"]),
		}, true
	default:
		return protocol.SysErrorArgs{}, false
	}
}

func extractUnknownCommand(args any) (protocol.UnknownCommandArgs, bool) {
	switch v := args.(type) {
	case protocol.UnknownCommandArgs:
		return v, true
	case map[string]any:
		return protocol.UnknownCommandArgs{
			Name: stringFromAny(v["name"]),
		}, true
	default:
		return protocol.UnknownCommandArgs{}, false
	}
}

func extractInternalError(args any) (protocol.InternalErrorArgs, bool) {
	switch v := args.(type) {
	case protocol.InternalErrorArgs:
		return v, true
	case map[string]any:
		return protocol.InternalErrorArgs{
			Error:     intFromAny(v["error"]),
			ErrorText: stringFromAny(v["error_text"]),
			DMX:       v["dmx"],
			Message:   stringFromAny(v["message"]),
		}, true
	default:
		return protocol.InternalErrorArgs{}, false
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

func extractFrameIDArgument(args any) int {
	if args == nil {
		return 0
	}
	m, ok := args.(map[string]any)
	if !ok {
		return 0
	}
	return intFromAny(m["frameId"])
}

func extractVariablesReferenceArgument(args any) int {
	if args == nil {
		return 0
	}
	m, ok := args.(map[string]any)
	if !ok {
		return 0
	}
	return intFromAny(m["variablesReference"])
}

func extractEvaluateArguments(args any) (evaluateArguments, bool) {
	typedArgs, ok := args.(map[string]any)
	if !ok {
		return evaluateArguments{}, false
	}
	expression := stringFromAny(typedArgs["expression"])
	if expression == "" {
		return evaluateArguments{}, false
	}
	context := stringFromAny(typedArgs["context"])
	if context == "" {
		context = "repl"
	}
	return evaluateArguments{
		expression: expression,
		frameID:    intFromAny(typedArgs["frameId"]),
		context:    context,
	}, true
}

func extractSourceArguments(args any) (sourceArguments, bool) {
	typedArgs, ok := args.(map[string]any)
	if !ok {
		return sourceArguments{}, false
	}
	parsed := sourceArguments{
		sourceReference: intFromAny(typedArgs["sourceReference"]),
	}
	if source, ok := typedArgs["source"].(map[string]any); ok {
		parsed.path = stringFromAny(source["path"])
		if parsed.sourceReference <= 0 {
			parsed.sourceReference = intFromAny(source["sourceReference"])
		}
	}
	if parsed.path == "" && parsed.sourceReference <= 0 {
		return sourceArguments{}, false
	}
	return parsed, true
}

func extractSetBreakpointsArguments(args any) (setBreakpointsArguments, bool) {
	typedArgs, ok := args.(map[string]any)
	if !ok {
		return setBreakpointsArguments{}, false
	}

	source, ok := typedArgs["source"].(map[string]any)
	if !ok {
		return setBreakpointsArguments{}, false
	}

	parsed := setBreakpointsArguments{
		path:            stringFromAny(source["path"]),
		sourceReference: intFromAny(source["sourceReference"]),
	}
	if parsed.path == "" && parsed.sourceReference <= 0 {
		return setBreakpointsArguments{}, false
	}

	parsed.lines = extractBreakpointLines(typedArgs)
	return parsed, true
}

func extractBreakpointLines(args map[string]any) []int {
	if rawBreakpoints, ok := args["breakpoints"]; ok {
		items, ok := rawBreakpoints.([]any)
		if !ok {
			return nil
		}

		lines := make([]int, 0, len(items))
		for _, raw := range items {
			line := 0
			if bp, ok := raw.(map[string]any); ok {
				line = intFromAny(bp["line"])
			}
			lines = append(lines, line)
		}
		return lines
	}

	rawLines, ok := args["lines"]
	if !ok {
		return nil
	}
	items, ok := rawLines.([]any)
	if !ok {
		return nil
	}

	lines := make([]int, 0, len(items))
	for _, raw := range items {
		lines = append(lines, intFromAny(raw))
	}
	return lines
}

func (s *Server) resolveTokenForSetBreakpoints(args setBreakpointsArguments) (int, bool) {
	if args.sourceReference > 0 {
		token, ok := s.tokenBySourceRef[args.sourceReference]
		return token, ok
	}

	sourceRef, ok := s.sourceRefByPath[args.path]
	if !ok {
		return 0, false
	}
	token, ok := s.tokenBySourceRef[sourceRef]
	return token, ok
}

func (s *Server) deferBreakpoints(args setBreakpointsArguments) {
	s.clearDeferredBreakpoints(args)
	lines := append([]int{}, args.lines...)
	if args.path != "" {
		s.pendingByPath[args.path] = lines
	}
	if args.sourceReference > 0 {
		s.pendingBySourceRef[args.sourceReference] = lines
	}
}

func (s *Server) clearDeferredBreakpoints(args setBreakpointsArguments) {
	if args.path != "" {
		delete(s.pendingByPath, args.path)
		if sourceRef, ok := s.sourceRefByPath[args.path]; ok {
			delete(s.pendingBySourceRef, sourceRef)
		}
	}
	if args.sourceReference > 0 {
		delete(s.pendingBySourceRef, args.sourceReference)
		if path, ok := s.pathBySourceRef[args.sourceReference]; ok {
			delete(s.pendingByPath, path)
		}
	}
}

func (s *Server) requestWindowLayoutSync() {
	if s.rideController == nil {
		return
	}
	_ = s.rideController.SendCommand("GetWindowLayout", map[string]any{})
}

func (s *Server) applyDeferredBreakpoints(token int) {
	if s.rideController == nil || token <= 0 {
		return
	}

	binding, ok := s.sourceByToken[token]
	if !ok {
		return
	}

	lines, ok := s.pendingBySourceRef[binding.sourceRef]
	if !ok {
		lines, ok = s.pendingByPath[binding.path]
		if !ok {
			return
		}
	}

	if err := s.rideController.SendCommand("SetLineAttributes", map[string]any{
		"win":     token,
		"stop":    zeroBasedLines(lines),
		"monitor": []int{},
		"trace":   []int{},
	}); err != nil {
		return
	}

	delete(s.pendingBySourceRef, binding.sourceRef)
	delete(s.pendingByPath, binding.path)
}

func buildBreakpointResponses(lines []int, verified bool) []Breakpoint {
	breakpoints := make([]Breakpoint, 0, len(lines))
	for _, line := range lines {
		breakpoints = append(breakpoints, Breakpoint{
			Verified: verified,
			Line:     line,
		})
	}
	return breakpoints
}

func zeroBasedLines(lines []int) []int {
	converted := make([]int, 0, len(lines))
	for _, line := range lines {
		if line <= 0 {
			continue
		}
		converted = append(converted, line-1)
	}
	return converted
}

func (s *Server) storeSourceText(path string, text []string) {
	if path == "" || text == nil {
		return
	}
	cloned := append([]string{}, text...)
	s.sourceTextByPath[path] = cloned
}

func (s *Server) refreshFrameSymbolsLocked(frameID int) {
	binding, hasBinding := s.sourceByToken[frameID]
	if !hasBinding || binding.path == "" {
		if _, exists := s.frameSymbols[frameID]; !exists {
			s.frameSymbols[frameID] = frameSymbolsState{
				order:   []string{},
				symbols: map[string]frameSymbol{},
			}
		}
		return
	}
	lines, ok := s.sourceTextByPath[binding.path]
	if !ok {
		if _, exists := s.frameSymbols[frameID]; !exists {
			s.frameSymbols[frameID] = frameSymbolsState{
				order:   []string{},
				symbols: map[string]frameSymbol{},
			}
		}
		return
	}

	order, localSet := extractVisibleSymbols(lines)
	if len(order) > maxLocalSymbolsPerFrame {
		order = append([]string{}, order[:maxLocalSymbolsPerFrame]...)
	}

	existing := s.frameSymbols[frameID]
	state := frameSymbolsState{
		order:   append([]string{}, order...),
		symbols: map[string]frameSymbol{},
	}
	scopeNeedsReset := len(existing.order) != len(order)
	if !scopeNeedsReset {
		for i := range order {
			if existing.order[i] != order[i] {
				scopeNeedsReset = true
				break
			}
		}
	}
	for _, name := range order {
		symbol := frameSymbol{
			name:    name,
			isLocal: localSet[name],
		}
		if existingSymbol, exists := existing.symbols[name]; exists {
			symbol.value = existingSymbol.value
			symbol.class = existingSymbol.class
			symbol.hasValue = existingSymbol.hasValue
			if existingSymbol.isLocal != symbol.isLocal {
				scopeNeedsReset = true
			}
		}
		state.symbols[name] = symbol
	}
	if scopeNeedsReset {
		if ref, exists := s.frameScopeRef[frameID]; exists {
			s.dropVariableReference(ref)
			delete(s.frameScopeRef, frameID)
		}
	}
	s.frameSymbols[frameID] = state
}

func (s *Server) prepareSymbolTipRequestsLocked(frameID int) []symbolTipRequest {
	if s.rideController == nil {
		return nil
	}
	if s.promptTypeSeen && s.promptType == 0 {
		return nil
	}
	state, ok := s.frameSymbols[frameID]
	if !ok || len(state.order) == 0 {
		return nil
	}

	requests := make([]symbolTipRequest, 0, len(state.order))
	for _, name := range state.order {
		symbol, exists := state.symbols[name]
		if !exists || symbol.hasValue {
			continue
		}

		token := s.nextSymbolTipToken
		s.nextSymbolTipToken++
		s.pendingSymbolTips[token] = pendingSymbolTip{
			frameID: frameID,
			name:    name,
		}
		requests = append(requests, symbolTipRequest{
			token: token,
			args: map[string]any{
				"win":       frameID,
				"line":      name,
				"pos":       utf8.RuneCountInString(name),
				"maxWidth":  maxLocalValuePreviewRunes,
				"maxHeight": maxLocalValueChildren,
				"token":     token,
			},
		})
	}
	return requests
}

func (s *Server) waitForSymbolTipRequests(frameID int, timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			s.mu.Lock()
			s.dropPendingSymbolTipsForFrame(frameID)
			s.mu.Unlock()
			return
		}

		s.mu.Lock()
		pending := false
		for _, request := range s.pendingSymbolTips {
			if request.frameID == frameID {
				pending = true
				break
			}
		}
		s.mu.Unlock()
		if !pending {
			return
		}
		time.Sleep(localsFetchPollInterval)
	}
}

func extractVisibleSymbols(lines []string) ([]string, map[string]bool) {
	locals := map[string]bool{}
	assigned := map[string]bool{}

	if len(lines) > 0 {
		parts := strings.Split(lines[0], ";")
		for i := 1; i < len(parts); i++ {
			name := strings.TrimSpace(parts[i])
			if isSymbolName(name) {
				locals[name] = true
			}
		}
	}

	for _, line := range lines {
		for _, name := range extractAssignedSymbols(line) {
			assigned[name] = true
		}
	}

	names := make([]string, 0, len(locals)+len(assigned))
	seen := map[string]bool{}
	for name := range locals {
		seen[name] = true
		names = append(names, name)
	}
	for name := range assigned {
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, locals
}

func extractAssignedSymbols(line string) []string {
	runes := []rune(line)
	names := make([]string, 0, 4)
	for i, r := range runes {
		if r != '←' {
			continue
		}
		start := i - 1
		for start >= 0 && isSymbolRune(runes[start]) {
			start--
		}
		start++
		if start >= i {
			continue
		}
		name := string(runes[start:i])
		if isSymbolName(name) {
			names = append(names, name)
		}
	}
	return names
}

func isSymbolName(name string) bool {
	if name == "" {
		return false
	}
	runes := []rune(name)
	if len(runes) == 0 {
		return false
	}
	for i, r := range runes {
		if !isSymbolRune(r) {
			return false
		}
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isSymbolRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '_', '⎕', '∆', '⍙':
		return true
	default:
		return false
	}
}

func extractSetPromptType(args any) (int, bool) {
	switch v := args.(type) {
	case protocol.SetPromptTypeArgs:
		return v.Type, true
	case map[string]any:
		return intFromAny(v["type"]), true
	default:
		return 0, false
	}
}

func extractValueTip(args any) (valueTipArgs, bool) {
	v, ok := args.(map[string]any)
	if !ok {
		return valueTipArgs{}, false
	}
	return valueTipArgs{
		tip:   stringSliceFromAny(v["tip"]),
		class: intFromAny(v["class"]),
		token: intFromAny(v["token"]),
	}, true
}

func (s *Server) dispatchValueTip(valueTip valueTipArgs) {
	waiter, ok := s.evaluateWaiters[valueTip.token]
	if ok {
		delete(s.evaluateWaiters, valueTip.token)
		select {
		case waiter <- evaluateResult{
			text:  strings.Join(valueTip.tip, "\n"),
			class: valueTip.class,
		}:
		default:
		}
		return
	}

	pending, exists := s.pendingSymbolTips[valueTip.token]
	if !exists {
		return
	}
	delete(s.pendingSymbolTips, valueTip.token)

	state, ok := s.frameSymbols[pending.frameID]
	if !ok {
		return
	}
	symbol, ok := state.symbols[pending.name]
	if !ok {
		return
	}
	symbol.value = strings.Join(valueTip.tip, "\n")
	symbol.class = valueTip.class
	symbol.hasValue = true
	state.symbols[pending.name] = symbol
	s.frameSymbols[pending.frameID] = state
}

func (s *Server) ensureScopeForFrame(frameID int) (int, bool) {
	if _, ok := s.tracerWindows[frameID]; !ok {
		return 0, false
	}
	if ref, ok := s.frameScopeRef[frameID]; ok {
		if _, exists := s.variablesByRef[ref]; exists {
			return ref, true
		}
	}

	frameVars := s.buildFrameVariables(frameID)
	scopeRef := s.allocateVariablesReference(frameVars)
	s.frameScopeRef[frameID] = scopeRef
	return scopeRef, true
}

func (s *Server) buildFrameVariables(frameID int) []Variable {
	frame := s.tracerWindows[frameID]
	threadID := frame.threadID
	threadName := ""
	if thread, ok := s.threadCache[threadID]; ok {
		threadName = thread.Name
	}
	if threadName == "" {
		threadName = fmt.Sprintf("Thread %d", threadID)
	}

	sourceBinding, hasSource := s.sourceByToken[frameID]
	sourcePath := ""
	sourceRef := 0
	if hasSource {
		sourcePath = sourceBinding.path
		sourceRef = sourceBinding.sourceRef
	}
	sourceChildren := []Variable{
		{Name: "path", Value: sourcePath, Type: "string", VariablesReference: 0},
		{Name: "sourceReference", Value: fmt.Sprintf("%d", sourceRef), Type: "number", VariablesReference: 0},
		{Name: "token", Value: fmt.Sprintf("%d", frameID), Type: "number", VariablesReference: 0},
	}
	if hasSource && sourceBinding.displayName != "" {
		sourceChildren = append(sourceChildren, Variable{
			Name:               "displayName",
			Value:              sourceBinding.displayName,
			Type:               "string",
			VariablesReference: 0,
		})
	}
	sourceChildrenRef := s.allocateVariablesReference(sourceChildren)

	threadChildrenRef := s.allocateVariablesReference([]Variable{
		{Name: "id", Value: fmt.Sprintf("%d", threadID), Type: "number", VariablesReference: 0},
		{Name: "name", Value: threadName, Type: "string", VariablesReference: 0},
	})

	siDescriptions := append([]string{}, s.siDescriptions[threadID]...)
	siChildren := make([]Variable, 0, len(siDescriptions))
	for i, description := range siDescriptions {
		siChildren = append(siChildren, Variable{
			Name:               fmt.Sprintf("[%d]", i),
			Value:              description,
			Type:               "string",
			VariablesReference: 0,
		})
	}
	siChildrenRef := s.allocateVariablesReference(siChildren)
	localsChildren, globalsChildren := s.buildFrameSymbolVariables(frameID)
	localsRef := s.allocateVariablesReference(localsChildren)

	variables := []Variable{
		{
			Name:               "locals",
			Value:              fmt.Sprintf("[%d]", len(localsChildren)),
			Type:               "array",
			VariablesReference: localsRef,
		},
		{Name: "frameName", Value: frame.name, Type: "string", VariablesReference: 0},
		{Name: "line", Value: fmt.Sprintf("%d", oneBased(frame.line)), Type: "number", VariablesReference: 0},
		{Name: "column", Value: fmt.Sprintf("%d", oneBased(frame.column)), Type: "number", VariablesReference: 0},
		{Name: "threadId", Value: fmt.Sprintf("%d", threadID), Type: "number", VariablesReference: 0},
		{Name: "threadName", Value: threadName, Type: "string", VariablesReference: 0},
		{Name: "thread", Value: "Object", Type: "object", VariablesReference: threadChildrenRef},
		{Name: "source", Value: "Object", Type: "object", VariablesReference: sourceChildrenRef},
		{Name: "siStack", Value: fmt.Sprintf("[%d]", len(siDescriptions)), Type: "array", VariablesReference: siChildrenRef},
	}
	if len(globalsChildren) > 0 {
		globalsRef := s.allocateVariablesReference(globalsChildren)
		variables = append([]Variable{{
			Name:               "globals",
			Value:              fmt.Sprintf("[%d]", len(globalsChildren)),
			Type:               "array",
			VariablesReference: globalsRef,
		}}, variables...)
	}
	return variables
}

func (s *Server) buildFrameSymbolVariables(frameID int) ([]Variable, []Variable) {
	state, ok := s.frameSymbols[frameID]
	if !ok {
		return []Variable{}, []Variable{}
	}
	locals := []Variable{}
	globals := []Variable{}
	for _, name := range state.order {
		symbol, exists := state.symbols[name]
		if !exists {
			continue
		}
		variable := s.buildInspectableSymbolVariable(symbol)
		if symbol.isLocal {
			locals = append(locals, variable)
		} else {
			globals = append(globals, variable)
		}
	}
	return locals, globals
}

func (s *Server) buildInspectableSymbolVariable(symbol frameSymbol) Variable {
	if !symbol.hasValue {
		return Variable{
			Name:               symbol.name,
			Value:              "(loading value)",
			Type:               "unknown",
			VariablesReference: 0,
		}
	}

	valueType := "string"
	if symbol.class != 0 {
		valueType = fmt.Sprintf("nameclass(%d)", symbol.class)
	}

	preview, children := buildValuePreviewAndChildren(symbol.value)
	childRef := 0
	if len(children) > 0 {
		childRef = s.allocateVariablesReference(children)
	}

	return Variable{
		Name:               symbol.name,
		Value:              preview,
		Type:               valueType,
		VariablesReference: childRef,
	}
}

func buildValuePreviewAndChildren(value string) (string, []Variable) {
	if value == "" {
		return "", []Variable{}
	}
	lines := strings.Split(value, "\n")
	firstLine := lines[0]
	preview := truncateRunes(firstLine, maxLocalValuePreviewRunes)
	if len(lines) > 1 {
		preview = fmt.Sprintf("%s … (+%d lines)", truncateRunes(firstLine, maxLocalValuePreviewRunes/2), len(lines)-1)
	}

	hasLongSingleLine := len(lines) == 1 && utf8.RuneCountInString(firstLine) > maxLocalValuePreviewRunes
	if len(lines) == 1 && !hasLongSingleLine {
		return preview, []Variable{}
	}

	children := make([]Variable, 0, maxLocalValueChildren+1)
	entries := lines
	if hasLongSingleLine {
		entries = chunkByRunes(firstLine, maxLocalValuePreviewRunes)
	}
	limit := len(entries)
	if limit > maxLocalValueChildren {
		limit = maxLocalValueChildren
	}
	for i := 0; i < limit; i++ {
		children = append(children, Variable{
			Name:               fmt.Sprintf("[%d]", i),
			Value:              truncateRunes(entries[i], maxLocalValuePreviewRunes),
			Type:               "string",
			VariablesReference: 0,
		})
	}
	if len(entries) > maxLocalValueChildren {
		children = append(children, Variable{
			Name:               "[...]",
			Value:              fmt.Sprintf("%d more entries", len(entries)-maxLocalValueChildren),
			Type:               "string",
			VariablesReference: 0,
		})
	}
	return preview, children
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func chunkByRunes(value string, chunkSize int) []string {
	if chunkSize <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	if len(runes) <= chunkSize {
		return []string{value}
	}
	chunks := make([]string, 0, len(runes)/chunkSize+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func (s *Server) allocateVariablesReference(variables []Variable) int {
	ref := s.nextVariablesRef
	s.nextVariablesRef++
	s.variablesByRef[ref] = cloneVariables(variables)
	return ref
}

func cloneVariables(variables []Variable) []Variable {
	if len(variables) == 0 {
		return []Variable{}
	}
	cloned := make([]Variable, len(variables))
	copy(cloned, variables)
	return cloned
}

func (s *Server) dropVariableReference(ref int) {
	if ref <= 0 {
		return
	}
	variables, ok := s.variablesByRef[ref]
	if !ok {
		return
	}
	delete(s.variablesByRef, ref)
	for _, variable := range variables {
		if variable.VariablesReference > 0 {
			s.dropVariableReference(variable.VariablesReference)
		}
	}
}

func (s *Server) dropPendingSymbolTipsForFrame(frameID int) {
	for token, pending := range s.pendingSymbolTips {
		if pending.frameID == frameID {
			delete(s.pendingSymbolTips, token)
		}
	}
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

func stringSliceFromAny(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func (s *Server) success(req Request) Response {
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
	}
}

func (s *Server) successWithBody(req Request, body any) Response {
	return Response{
		RequestSeq: req.Seq,
		Command:    req.Command,
		Success:    true,
		Body:       body,
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
