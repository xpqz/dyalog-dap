package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func ReadPayload(reader *bufio.Reader) ([]byte, error) {
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

func WritePayload(w io.Writer, message any) error {
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
