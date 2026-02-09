package transport

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// TrafficDirection indicates whether protocol traffic was inbound or outbound.
type TrafficDirection string

const (
	// DirectionInbound represents payloads read from the interpreter.
	DirectionInbound TrafficDirection = "inbound"
	// DirectionOutbound represents payloads sent to the interpreter.
	DirectionOutbound TrafficDirection = "outbound"
)

// TrafficLogEntry is a structured protocol traffic log record.
type TrafficLogEntry struct {
	Timestamp time.Time        `json:"timestamp"`
	Direction TrafficDirection `json:"direction"`
	Payload   string           `json:"payload"`
}

// TrafficLogger records protocol payload traffic.
type TrafficLogger interface {
	LogTraffic(direction TrafficDirection, payload string)
}

// JSONLTrafficLogger writes timestamped traffic records as JSON Lines.
type JSONLTrafficLogger struct {
	mu  sync.Mutex
	enc *json.Encoder
	now func() time.Time
}

// NewJSONLTrafficLogger creates a structured JSON-lines traffic logger.
func NewJSONLTrafficLogger(w io.Writer) *JSONLTrafficLogger {
	return &JSONLTrafficLogger{
		enc: json.NewEncoder(w),
		now: time.Now,
	}
}

// LogTraffic records one traffic entry.
func (l *JSONLTrafficLogger) LogTraffic(direction TrafficDirection, payload string) {
	if l == nil || l.enc == nil {
		return
	}

	entry := TrafficLogEntry{
		Timestamp: l.now().UTC(),
		Direction: direction,
		Payload:   payload,
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.enc.Encode(entry)
}
