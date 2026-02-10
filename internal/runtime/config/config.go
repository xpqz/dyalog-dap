package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/stefan/lsp-dap/internal/integration/harness"
	"github.com/stefan/lsp-dap/internal/support/decode"
)

func FromRequest(requestCommand string, arguments any) (harness.Config, error) {
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
