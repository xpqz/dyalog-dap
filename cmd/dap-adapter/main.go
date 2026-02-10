package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/stefan/lsp-dap/internal/dap/adapter"
	daptransport "github.com/stefan/lsp-dap/internal/dap/transport"
	"github.com/stefan/lsp-dap/internal/integration/harness"
	"github.com/stefan/lsp-dap/internal/ride/protocol"
	"github.com/stefan/lsp-dap/internal/ride/sessionstate"
	runtimeconfig "github.com/stefan/lsp-dap/internal/runtime/config"
	"github.com/stefan/lsp-dap/internal/support/decode"
)

const runtimeStopWaitTimeout = 3 * time.Second
const defaultLinkExpression = "]LINK.Create # ."
const postConfigurationCommandTimeout = 30 * time.Second

func main() {
	if err := run(context.Background(), os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dap-adapter: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	reader := bufio.NewReader(stdin)
	writer := newDAPWriter(stdout)
	server := adapter.NewServer()
	runtime := newRideRuntime(server, writer)
	defer runtime.stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		payload, err := readDAPPayload(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var request dapRequestMessage
		if err := json.Unmarshal(payload, &request); err != nil {
			return err
		}
		if request.Type != "request" {
			continue
		}

		if (request.Command == "launch" || request.Command == "attach") &&
			server.CanLaunchOrAttach() &&
			!runtime.started() {
			if err := runtime.start(ctx, request.Command, request.Arguments); err != nil {
				response := adapter.Response{
					RequestSeq: request.Seq,
					Command:    request.Command,
					Success:    false,
					Message:    err.Error(),
				}
				if writeErr := writer.writeResponse(response); writeErr != nil {
					return writeErr
				}
				continue
			}
		}

		response, events := server.HandleRequest(adapter.Request{
			Seq:       request.Seq,
			Command:   request.Command,
			Arguments: request.Arguments,
		})
		if request.Command == "configurationDone" && response.Success {
			if err := runtime.executeConfiguredLaunchExpression(); err != nil {
				response = adapter.Response{
					RequestSeq: request.Seq,
					Command:    request.Command,
					Success:    false,
					Message:    err.Error(),
				}
				events = nil
			}
		}
		if err := writer.writeResponse(response); err != nil {
			return err
		}
		for _, event := range events {
			if err := writer.writeEvent(event); err != nil {
				return err
			}
		}

		if (request.Command == "disconnect" || request.Command == "terminate") && response.Success {
			if err := runtime.stop(); err != nil {
				_, _ = fmt.Fprintf(stderr, "dap-adapter runtime stop: %v\n", err)
			}
			return nil
		}
	}
}

type dapRequestMessage struct {
	Seq       int `json:"seq"`
	Type      string
	Command   string `json:"command"`
	Arguments any    `json:"arguments,omitempty"`
}

type dapResponseMessage struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"`
	RequestSeq int    `json:"request_seq"`
	Command    string `json:"command"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	Body       any    `json:"body,omitempty"`
}

type dapEventMessage struct {
	Seq   int    `json:"seq"`
	Type  string `json:"type"`
	Event string `json:"event"`
	Body  any    `json:"body,omitempty"`
}

type dapWriter struct {
	mu      sync.Mutex
	nextSeq int
	out     io.Writer
}

func newDAPWriter(out io.Writer) *dapWriter {
	return &dapWriter{
		nextSeq: 1,
		out:     out,
	}
}

func (w *dapWriter) writeResponse(response adapter.Response) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	message := dapResponseMessage{
		Seq:        w.nextSeq,
		Type:       "response",
		RequestSeq: response.RequestSeq,
		Command:    response.Command,
		Success:    response.Success,
		Message:    response.Message,
		Body:       response.Body,
	}
	w.nextSeq++

	return writeDAPPayload(w.out, message)
}

func (w *dapWriter) writeEvent(event adapter.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	message := dapEventMessage{
		Seq:   w.nextSeq,
		Type:  "event",
		Event: event.Event,
		Body:  event.Body,
	}
	w.nextSeq++

	return writeDAPPayload(w.out, message)
}

type rideRuntime struct {
	mu          sync.Mutex
	server      *adapter.Server
	writer      *dapWriter
	harness     *harness.Harness
	dispatcher  *sessionstate.Dispatcher
	requestType string
	autoLink    bool
	linkExpr    string
	launchExpr  string
	launchRan   bool
	unsubscribe func()
	cancel      context.CancelFunc
	runDone     chan struct{}
	bridgeDone  chan struct{}
}

func newRideRuntime(server *adapter.Server, writer *dapWriter) *rideRuntime {
	return &rideRuntime{
		server: server,
		writer: writer,
	}
}

func (r *rideRuntime) started() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dispatcher != nil
}

func (r *rideRuntime) start(ctx context.Context, requestCommand string, args any) error {
	r.mu.Lock()
	if r.dispatcher != nil {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	cfg, err := runtimeConfigFrom(requestCommand, args)
	if err != nil {
		return err
	}
	autoLink := runtimeAutoLinkFrom(requestCommand, args)
	linkExpr := runtimeLinkExpressionFrom(requestCommand, args)
	launchExpr := runtimeLaunchExpressionFrom(args)

	h := harness.New(cfg)
	client, err := h.Start(ctx, "dap-adapter")
	if err != nil {
		return err
	}

	dispatcher := sessionstate.NewDispatcher(client, protocol.NewCodec())
	events, unsubscribe := dispatcher.Subscribe(1024)
	r.server.SetRideController(dispatcher)

	runCtx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		dispatcher.Run(runCtx)
		close(runDone)
	}()

	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case <-runCtx.Done():
				return
			case event := <-events:
				outbound := r.server.HandleRidePayload(event)
				for _, dapEvent := range outbound {
					_ = r.writer.writeEvent(dapEvent)
				}
			}
		}
	}()

	r.mu.Lock()
	r.harness = h
	r.dispatcher = dispatcher
	r.requestType = requestCommand
	r.autoLink = autoLink
	r.linkExpr = linkExpr
	r.launchExpr = launchExpr
	r.launchRan = false
	r.unsubscribe = unsubscribe
	r.cancel = cancel
	r.runDone = runDone
	r.bridgeDone = bridgeDone
	r.mu.Unlock()

	return nil
}

func (r *rideRuntime) stop() error {
	r.mu.Lock()
	if r.dispatcher == nil {
		r.mu.Unlock()
		return nil
	}

	cancel := r.cancel
	runDone := r.runDone
	bridgeDone := r.bridgeDone
	unsubscribe := r.unsubscribe
	h := r.harness

	r.harness = nil
	r.dispatcher = nil
	r.requestType = ""
	r.autoLink = false
	r.linkExpr = ""
	r.launchExpr = ""
	r.launchRan = false
	r.unsubscribe = nil
	r.cancel = nil
	r.runDone = nil
	r.bridgeDone = nil
	r.server.SetRideController(nil)
	r.mu.Unlock()

	var errs []error

	if unsubscribe != nil {
		unsubscribe()
	}
	if h != nil {
		if err := h.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close harness: %w", err))
		}
	}
	if cancel != nil {
		cancel()
	}
	if err := waitForDone(runDone, runtimeStopWaitTimeout, "RIDE dispatcher"); err != nil {
		errs = append(errs, err)
	}
	if err := waitForDone(bridgeDone, runtimeStopWaitTimeout, "DAP bridge"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (r *rideRuntime) executeConfiguredLaunchExpression() error {
	r.mu.Lock()
	dispatcher := r.dispatcher
	requestType := r.requestType
	autoLink := r.autoLink
	linkExpr := strings.TrimSpace(r.linkExpr)
	expr := strings.TrimSpace(r.launchExpr)
	alreadyRan := r.launchRan
	if dispatcher == nil {
		r.mu.Unlock()
		return errors.New("RIDE runtime is not active")
	}
	if alreadyRan {
		r.mu.Unlock()
		return nil
	}
	needsLink := requestType == "launch" && autoLink && linkExpr != ""
	needsLaunch := expr != ""
	if !needsLink && !needsLaunch {
		r.launchRan = true
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if needsLink {
		if err := executeRuntimeCommand(dispatcher, linkExpr, true); err != nil {
			return fmt.Errorf("failed to execute linkExpression: %w", err)
		}
	}

	if needsLaunch {
		if err := executeRuntimeCommand(dispatcher, expr, false); err != nil {
			return fmt.Errorf("failed to execute launchExpression: %w", err)
		}
	}

	r.mu.Lock()
	r.launchRan = true
	r.mu.Unlock()
	return nil
}

func executeRuntimeCommand(dispatcher *sessionstate.Dispatcher, text string, waitForCompletion bool) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	var (
		events      <-chan protocol.DecodedPayload
		unsubscribe func()
	)
	if waitForCompletion {
		events, unsubscribe = dispatcher.Subscribe(1024)
	}
	if unsubscribe != nil {
		defer unsubscribe()
	}

	if !strings.HasSuffix(trimmed, "\n") {
		trimmed += "\n"
	}
	if err := dispatcher.SendCommand("Execute", map[string]any{
		"text":  trimmed,
		"trace": 0,
	}); err != nil {
		return err
	}

	if !waitForCompletion || events == nil {
		return nil
	}

	return waitForExecuteCompletion(events, postConfigurationCommandTimeout)
}

func waitForExecuteCompletion(events <-chan protocol.DecodedPayload, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	sawBusyPrompt := false
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for command completion after %s", timeout)
		case payload, ok := <-events:
			if !ok {
				return errors.New("dispatcher event subscription closed while waiting for command completion")
			}
			if payload.Kind != protocol.KindCommand {
				continue
			}
			switch payload.Command {
			case "SetPromptType":
				promptType, ok := promptTypeFromArgs(payload.Args)
				if !ok {
					continue
				}
				if promptType == 0 {
					sawBusyPrompt = true
					continue
				}
				if sawBusyPrompt {
					return nil
				}
			case "HadError":
				code, text := hadErrorFromArgs(payload.Args)
				if text != "" {
					return fmt.Errorf("RIDE reported HadError %d: %s", code, text)
				}
				return fmt.Errorf("RIDE reported HadError %d", code)
			}
		}
	}
}

func promptTypeFromArgs(args any) (int, bool) {
	switch typed := args.(type) {
	case protocol.SetPromptTypeArgs:
		return typed.Type, true
	case map[string]any:
		return decode.Int(typed["type"])
	default:
		return 0, false
	}
}

func hadErrorFromArgs(args any) (int, string) {
	switch typed := args.(type) {
	case protocol.HadErrorArgs:
		return typed.Error, typed.ErrorText
	case map[string]any:
		code, _ := decode.Int(typed["error"])
		text := strings.TrimSpace(decode.StringOrEmpty(typed["error_text"]))
		return code, text
	default:
		return 0, ""
	}
}

func waitForDone(done <-chan struct{}, timeout time.Duration, component string) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for %s shutdown after %s", component, timeout)
	}
}

func runtimeConfigFrom(requestCommand string, arguments any) (harness.Config, error) {
	return runtimeconfig.FromRequest(requestCommand, arguments)
}

func runtimeLaunchExpressionFrom(arguments any) string {
	argsMap, ok := arguments.(map[string]any)
	if !ok {
		return ""
	}
	text, _ := decode.NonEmptyTrimmedStringFromMap(argsMap, "launchExpression")
	return text
}

func runtimeAutoLinkFrom(requestCommand string, arguments any) bool {
	if requestCommand != "launch" {
		return false
	}
	argsMap, ok := arguments.(map[string]any)
	if !ok {
		return true
	}
	if value, exists := argsMap["autoLink"]; exists {
		if parsed, ok := decode.Bool(value); ok {
			return parsed
		}
	}
	return true
}

func runtimeLinkExpressionFrom(requestCommand string, arguments any) string {
	if requestCommand != "launch" {
		return ""
	}
	argsMap, ok := arguments.(map[string]any)
	if !ok {
		return defaultLinkExpression
	}
	if text, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "linkExpression"); ok {
		return text
	}
	return defaultLinkExpression
}

func readDAPPayload(reader *bufio.Reader) ([]byte, error) {
	return daptransport.ReadPayload(reader)
}

func writeDAPPayload(w io.Writer, message any) error {
	return daptransport.WritePayload(w, message)
}
