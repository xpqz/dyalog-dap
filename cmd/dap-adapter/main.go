package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stefan/lsp-dap/internal/dap/adapter"
	"github.com/stefan/lsp-dap/internal/integration/harness"
	"github.com/stefan/lsp-dap/internal/ride/protocol"
	"github.com/stefan/lsp-dap/internal/ride/sessionstate"
	"github.com/stefan/lsp-dap/internal/support/decode"
)

const runtimeStopWaitTimeout = 3 * time.Second

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
	expr := strings.TrimSpace(r.launchExpr)
	alreadyRan := r.launchRan
	if dispatcher == nil {
		r.mu.Unlock()
		return errors.New("RIDE runtime is not active")
	}
	if alreadyRan || expr == "" {
		r.mu.Unlock()
		return nil
	}
	r.launchRan = true
	r.mu.Unlock()

	text := expr
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := dispatcher.SendCommand("Execute", map[string]any{
		"text":  text,
		"trace": 0,
	}); err != nil {
		r.mu.Lock()
		r.launchRan = false
		r.mu.Unlock()
		return fmt.Errorf("failed to execute launchExpression: %w", err)
	}
	return nil
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
	cfg := harness.ConfigFromEnv()
	explicitLaunchSetting := false

	argsMap, ok := arguments.(map[string]any)
	if !ok {
		if requestCommand == "attach" {
			cfg.LaunchCommand = ""
		}
		if cfg.RideAddr == "" {
			return cfg, errors.New("launch/attach requires rideAddr (or DYALOG_RIDE_ADDR)")
		}
		return cfg, nil
	}

	if rideAddr, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "rideAddr"); ok {
		cfg.RideAddr = rideAddr
	}
	if rideAddr, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "address"); ok && cfg.RideAddr == "" {
		cfg.RideAddr = rideAddr
	}
	if launchCommand, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "rideLaunchCommand"); ok {
		cfg.LaunchCommand = launchCommand
		explicitLaunchSetting = true
	}
	if launchCommand, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "rideLaunch"); ok && cfg.LaunchCommand == "" {
		cfg.LaunchCommand = launchCommand
		explicitLaunchSetting = true
	}
	if transcriptsDir, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "rideTranscriptsDir"); ok {
		cfg.TranscriptDir = transcriptsDir
	}
	if timeoutText, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "rideConnectTimeout"); ok {
		timeout, err := time.ParseDuration(timeoutText)
		if err != nil {
			return cfg, fmt.Errorf("invalid rideConnectTimeout %q: %w", timeoutText, err)
		}
		cfg.ConnectTimeout = timeout
	}
	if timeoutMs, ok := decode.IntFromMapTextOrNumber(argsMap, "rideConnectTimeoutMs"); ok && timeoutMs > 0 {
		cfg.ConnectTimeout = time.Duration(timeoutMs) * time.Millisecond
	}
	if dyalogBin, ok := decode.NonEmptyTrimmedStringFromMap(argsMap, "dyalogBin"); ok && cfg.LaunchCommand == "" && cfg.RideAddr != "" {
		command, err := harness.DyalogServeLaunchCommand(cfg.RideAddr, dyalogBin)
		if err != nil {
			return cfg, err
		}
		cfg.LaunchCommand = command
		explicitLaunchSetting = true
	}

	if requestCommand == "attach" {
		if explicitLaunchSetting {
			return cfg, errors.New("attach does not support adapter-owned launch; use launch request for rideLaunchCommand/dyalogBin")
		}
		// Attach is connect-only and must not inherit process ownership from environment launch settings.
		cfg.LaunchCommand = ""
	}

	if cfg.RideAddr == "" {
		return cfg, errors.New("launch/attach requires rideAddr (or DYALOG_RIDE_ADDR)")
	}
	return cfg, nil
}

func runtimeLaunchExpressionFrom(arguments any) string {
	argsMap, ok := arguments.(map[string]any)
	if !ok {
		return ""
	}
	text, _ := decode.NonEmptyTrimmedStringFromMap(argsMap, "launchExpression")
	return text
}
func readDAPPayload(reader *bufio.Reader) ([]byte, error) {
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		colon := strings.Index(trimmed, ":")
		if colon < 0 {
			return nil, fmt.Errorf("invalid DAP header line %q", trimmed)
		}
		key := strings.ToLower(strings.TrimSpace(trimmed[:colon]))
		value := strings.TrimSpace(trimmed[colon+1:])
		headers[key] = value
	}

	rawLength, ok := headers["content-length"]
	if !ok {
		return nil, errors.New("missing Content-Length header")
	}
	length, err := strconv.Atoi(rawLength)
	if err != nil || length < 0 {
		return nil, fmt.Errorf("invalid Content-Length %q", rawLength)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeDAPPayload(w io.Writer, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
