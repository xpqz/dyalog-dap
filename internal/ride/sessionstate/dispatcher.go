package sessionstate

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/stefan/lsp-dap/internal/ride/protocol"
)

// Transport is the minimal payload I/O surface required by the dispatcher.
type Transport interface {
	ReadPayload() (string, error)
	WritePayload(payload string) error
}

type outboundCommand struct {
	name string
	args any
}

// Dispatcher owns the single-reader receive loop and prompt-aware outbound gating.
type Dispatcher struct {
	transport Transport
	codec     *protocol.Codec

	mu             sync.Mutex
	promptType     int
	promptTypeSeen bool
	queue          []outboundCommand
	subscribers    map[int]chan protocol.DecodedPayload
	nextSubID      int
	busyAllowList  map[string]struct{}
}

// NewDispatcher creates a session dispatcher over transport and codec.
func NewDispatcher(transport Transport, codec *protocol.Codec) *Dispatcher {
	if codec == nil {
		codec = protocol.NewCodec()
	}
	return &Dispatcher{
		transport:   transport,
		codec:       codec,
		subscribers: map[int]chan protocol.DecodedPayload{},
		busyAllowList: map[string]struct{}{
			"WeakInterrupt":    {},
			"StrongInterrupt":  {},
			"SaveChanges":      {},
			"ReplySaveChanges": {},
			"CloseWindow":      {},
		},
	}
}

// Run starts the single-reader receive loop.
func (d *Dispatcher) Run(ctx context.Context) {
	if d.transport == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		payload, err := d.transport.ReadPayload()
		if err != nil {
			// EOF indicates transport shutdown, which is a normal stop condition.
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}

		decoded, err := d.codec.DecodePayload(payload)
		if err != nil {
			continue
		}

		d.handleInbound(decoded)
	}
}

// Subscribe registers an event subscriber channel.
func (d *Dispatcher) Subscribe(buffer int) (<-chan protocol.DecodedPayload, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan protocol.DecodedPayload, buffer)

	d.mu.Lock()
	id := d.nextSubID
	d.nextSubID++
	d.subscribers[id] = ch
	d.mu.Unlock()

	unsubscribe := func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		delete(d.subscribers, id)
	}

	return ch, unsubscribe
}

// PromptType returns the most recently observed prompt type and whether one has been seen.
func (d *Dispatcher) PromptType() (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.promptType, d.promptTypeSeen
}

// SendCommand sends a command immediately when allowed, or queues it while promptType=0.
func (d *Dispatcher) SendCommand(command string, args any) error {
	if d.transport == nil {
		return errors.New("dispatcher has no transport")
	}

	cmd := outboundCommand{name: command, args: args}

	d.mu.Lock()
	busy := d.promptTypeSeen && d.promptType == 0
	if busy && !d.isAllowedWhileBusy(command) {
		d.queue = append(d.queue, cmd)
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	return d.writeCommand(cmd)
}

func (d *Dispatcher) isAllowedWhileBusy(command string) bool {
	if strings.HasPrefix(command, "Reply") {
		return true
	}
	_, ok := d.busyAllowList[command]
	return ok
}

func (d *Dispatcher) handleInbound(decoded protocol.DecodedPayload) {
	if decoded.Kind == protocol.KindCommand && decoded.Command == "SetPromptType" {
		if promptType, ok := extractPromptType(decoded.Args); ok {
			d.handlePromptType(promptType)
		}
	}
	d.publish(decoded)
}

func (d *Dispatcher) handlePromptType(promptType int) {
	var queued []outboundCommand

	d.mu.Lock()
	wasBusy := d.promptTypeSeen && d.promptType == 0
	d.promptType = promptType
	d.promptTypeSeen = true

	if wasBusy && promptType != 0 && len(d.queue) > 0 {
		queued = append(queued, d.queue...)
		d.queue = nil
	}
	d.mu.Unlock()

	for _, cmd := range queued {
		if err := d.writeCommand(cmd); err != nil {
			// If flush fails, preserve order by prepending failed and remaining commands.
			d.mu.Lock()
			remainder := append([]outboundCommand{}, queued...)
			d.queue = append(remainder, d.queue...)
			d.mu.Unlock()
			return
		}
		queued = queued[1:]
	}
}

func (d *Dispatcher) writeCommand(cmd outboundCommand) error {
	payload, err := d.codec.EncodeCommand(cmd.name, cmd.args)
	if err != nil {
		return err
	}
	return d.transport.WritePayload(payload)
}

func (d *Dispatcher) publish(decoded protocol.DecodedPayload) {
	type subscriber struct {
		id int
		ch chan protocol.DecodedPayload
	}

	d.mu.Lock()
	subs := make([]subscriber, 0, len(d.subscribers))
	for id, ch := range d.subscribers {
		subs = append(subs, subscriber{id: id, ch: ch})
	}
	d.mu.Unlock()

	for _, subscriber := range subs {
		if d.tryPublish(subscriber.ch, decoded) {
			continue
		}

		d.mu.Lock()
		if current, ok := d.subscribers[subscriber.id]; ok && current == subscriber.ch {
			delete(d.subscribers, subscriber.id)
		}
		d.mu.Unlock()
	}
}

func (d *Dispatcher) tryPublish(ch chan protocol.DecodedPayload, decoded protocol.DecodedPayload) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- decoded:
	default:
	}
	return true
}

func extractPromptType(args any) (int, bool) {
	switch v := args.(type) {
	case protocol.SetPromptTypeArgs:
		return v.Type, true
	case map[string]any:
		raw, ok := v["type"]
		if !ok {
			return 0, false
		}
		return toInt(raw)
	default:
		return 0, false
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
