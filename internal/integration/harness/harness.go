package harness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/stefan/lsp-dap/internal/ride/transport"
)

const (
	defaultTranscriptDir  = "artifacts/integration"
	defaultConnectTimeout = 10 * time.Second
	launchStopTimeout     = 1500 * time.Millisecond
)

var (
	// ErrMissingRideAddr indicates no RIDE endpoint is configured for harness startup.
	ErrMissingRideAddr = errors.New("missing DYALOG_RIDE_ADDR")
)

// Config describes integration harness runtime settings.
type Config struct {
	RideAddr       string
	LaunchCommand  string
	ConnectTimeout time.Duration
	TranscriptDir  string
}

// ConfigFromEnv loads harness settings from environment variables.
func ConfigFromEnv() Config {
	cfg := Config{
		RideAddr:      os.Getenv("DYALOG_RIDE_ADDR"),
		LaunchCommand: os.Getenv("DYALOG_RIDE_LAUNCH"),
		TranscriptDir: os.Getenv("DYALOG_RIDE_TRANSCRIPTS_DIR"),
	}
	if cfg.TranscriptDir == "" {
		cfg.TranscriptDir = defaultTranscriptDir
	}
	cfg.ConnectTimeout = parseDurationEnv("DYALOG_RIDE_CONNECT_TIMEOUT", defaultConnectTimeout)
	return cfg
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// Harness manages a live integration session and protocol transcript artifacts.
type Harness struct {
	cfg Config

	client         *transport.Client
	launchCmd      *exec.Cmd
	transcriptPath string
	transcriptFile *os.File
}

// New creates a harness instance from config.
func New(cfg Config) *Harness {
	if cfg.TranscriptDir == "" {
		cfg.TranscriptDir = defaultTranscriptDir
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = defaultConnectTimeout
	}
	return &Harness{cfg: cfg}
}

// Start launches/connects to RIDE, initializes protocol session, and starts transcript logging.
func (h *Harness) Start(ctx context.Context, testName string) (*transport.Client, error) {
	if h.client != nil {
		return h.client, nil
	}
	if h.cfg.RideAddr == "" {
		return nil, ErrMissingRideAddr
	}

	if err := h.startLaunchCommand(ctx); err != nil {
		return nil, err
	}

	conn, err := waitForDial(ctx, h.cfg.RideAddr, h.cfg.ConnectTimeout)
	if err != nil {
		h.stopLaunchCommand()
		return nil, fmt.Errorf("dial RIDE endpoint: %w", err)
	}

	transcriptFile, transcriptPath, err := openTranscript(h.cfg.TranscriptDir, testName)
	if err != nil {
		_ = conn.Close()
		h.stopLaunchCommand()
		return nil, err
	}

	client := transport.NewClient()
	client.AttachConn(conn)
	client.SetTrafficLogger(transport.NewJSONLTrafficLogger(transcriptFile))

	if err := client.InitializeSession(); err != nil {
		_ = client.Close()
		_ = transcriptFile.Close()
		h.stopLaunchCommand()
		return nil, fmt.Errorf("initialize session: %w", err)
	}

	h.client = client
	h.transcriptFile = transcriptFile
	h.transcriptPath = transcriptPath
	return client, nil
}

// TranscriptPath returns the JSONL protocol transcript path for the current harness session.
func (h *Harness) TranscriptPath() string {
	return h.transcriptPath
}

// Close tears down client connection, transcript file, and launched process if present.
func (h *Harness) Close() error {
	var firstErr error

	if h.client != nil {
		if err := h.client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.client = nil
	}

	if h.transcriptFile != nil {
		if err := h.transcriptFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.transcriptFile = nil
	}

	if err := h.stopLaunchCommand(); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

func (h *Harness) startLaunchCommand(ctx context.Context) error {
	if h.cfg.LaunchCommand == "" || h.launchCmd != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, "sh", "-lc", h.cfg.LaunchCommand)
	setLaunchProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start launch command: %w", err)
	}
	h.launchCmd = cmd
	return nil
}

func (h *Harness) stopLaunchCommand() error {
	if h.launchCmd == nil {
		return nil
	}
	cmd := h.launchCmd
	h.launchCmd = nil

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	if cmd.Process != nil && cmd.ProcessState == nil {
		_ = terminateProcessTree(cmd)
		select {
		case err := <-waitCh:
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					// Expected when process group termination is initiated by harness cleanup.
					return nil
				}
				return err
			}
			return nil
		case <-time.After(launchStopTimeout):
			_ = killProcessTree(cmd)
		}
	}

	if err := <-waitCh; err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Expected when process group termination is initiated by harness cleanup.
			return nil
		}
		return err
	}
	return nil
}

func waitForDial(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(75 * time.Millisecond)
	}
}

func openTranscript(dir, testName string) (*os.File, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create transcript dir: %w", err)
	}

	safeName := sanitizeTestName(testName)
	fileName := fmt.Sprintf("%s-%d.jsonl", safeName, time.Now().UnixNano())
	path := filepath.Join(dir, fileName)

	f, err := os.Create(path)
	if err != nil {
		return nil, "", fmt.Errorf("create transcript file: %w", err)
	}
	return f, path, nil
}

var unsafeNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeTestName(name string) string {
	safe := unsafeNameChars.ReplaceAllString(name, "-")
	if safe == "" {
		return "integration"
	}
	return safe
}
