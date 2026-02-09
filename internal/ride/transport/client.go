package transport

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	rideMagic               = "RIDE"
	protocolVersion         = "2"
	supportedProtocolsFrame = "SupportedProtocols=" + protocolVersion
	usingProtocolFrame      = "UsingProtocol=" + protocolVersion
)

var (
	// ErrNoConnection indicates the client has not been attached to a socket.
	ErrNoConnection = errors.New("no connection attached")
	// ErrInvalidMagic indicates the frame did not contain the RIDE magic bytes.
	ErrInvalidMagic = errors.New("invalid RIDE frame magic")
)

// Client manages low-level RIDE transport interactions.
type Client struct {
	mu            sync.Mutex
	conn          net.Conn
	rd            *bufio.Reader
	trafficLogger TrafficLogger
}

// NewClient creates a transport client.
func NewClient() *Client {
	return &Client{}
}

// AttachConn attaches an active network connection to the client.
func (c *Client) AttachConn(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
	c.rd = bufio.NewReader(conn)
}

// SetTrafficLogger enables structured inbound/outbound payload logging.
func (c *Client) SetTrafficLogger(logger TrafficLogger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trafficLogger = logger
}

// WritePayload writes one framed RIDE payload.
func (c *Client) WritePayload(payload string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return ErrNoConnection
	}

	frameLen := uint32(len(payload) + 8)
	if err := binary.Write(c.conn, binary.BigEndian, frameLen); err != nil {
		return fmt.Errorf("write frame length: %w", err)
	}
	if _, err := io.WriteString(c.conn, rideMagic); err != nil {
		return fmt.Errorf("write frame magic: %w", err)
	}
	if _, err := io.WriteString(c.conn, payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	if c.trafficLogger != nil {
		c.trafficLogger.LogTraffic(DirectionOutbound, payload)
	}
	return nil
}

// ReadPayload reads one framed RIDE payload.
func (c *Client) ReadPayload() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rd == nil {
		return "", ErrNoConnection
	}

	var frameLen uint32
	if err := binary.Read(c.rd, binary.BigEndian, &frameLen); err != nil {
		return "", fmt.Errorf("read frame length: %w", err)
	}
	if frameLen < 8 {
		return "", fmt.Errorf("invalid frame length %d", frameLen)
	}

	body := make([]byte, int(frameLen)-4)
	if _, err := io.ReadFull(c.rd, body); err != nil {
		return "", fmt.Errorf("read frame body: %w", err)
	}
	if string(body[:4]) != rideMagic {
		return "", ErrInvalidMagic
	}
	payload := string(body[4:])
	if c.trafficLogger != nil {
		c.trafficLogger.LogTraffic(DirectionInbound, payload)
	}
	return payload, nil
}

// WriteCommand marshals and writes a command payload.
func (c *Client) WriteCommand(command string, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal([]any{command, args})
	if err != nil {
		return fmt.Errorf("marshal command %q: %w", command, err)
	}
	return c.WritePayload(string(payload))
}

// InitializeSession performs protocol v2 handshake and startup commands.
func (c *Client) InitializeSession() error {
	first, err := c.ReadPayload()
	if err != nil {
		return fmt.Errorf("read handshake supported protocols: %w", err)
	}
	if first != supportedProtocolsFrame {
		return fmt.Errorf("unexpected supported protocols payload %q", first)
	}

	if err := c.WritePayload(supportedProtocolsFrame); err != nil {
		return fmt.Errorf("write handshake supported protocols: %w", err)
	}
	if err := c.WritePayload(usingProtocolFrame); err != nil {
		return fmt.Errorf("write handshake using protocol: %w", err)
	}

	second, err := c.ReadPayload()
	if err != nil {
		return fmt.Errorf("read handshake using protocol: %w", err)
	}
	if second != usingProtocolFrame {
		return fmt.Errorf("unexpected using protocol payload %q", second)
	}

	if err := c.WriteCommand("Identify", map[string]any{
		"apiVersion": 1,
		"identity":   1,
	}); err != nil {
		return fmt.Errorf("send Identify: %w", err)
	}
	if err := c.WriteCommand("Connect", map[string]any{
		"remoteId": 2,
	}); err != nil {
		return fmt.Errorf("send Connect: %w", err)
	}
	if err := c.WriteCommand("GetWindowLayout", map[string]any{}); err != nil {
		return fmt.Errorf("send GetWindowLayout: %w", err)
	}

	return nil
}
