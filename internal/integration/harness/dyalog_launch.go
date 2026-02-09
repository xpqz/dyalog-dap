package harness

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// DyalogServeLaunchCommand builds a launch command that starts Dyalog in RIDE SERVE mode.
// The command form follows existing reference clients: RIDE_INIT=SERVE:*:<port> dyalog +s -q
func DyalogServeLaunchCommand(addr, executable string) (string, error) {
	if executable == "" {
		executable = "dyalog"
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("parse host:port address %q: %w", addr, err)
	}
	if port == "" {
		return "", fmt.Errorf("parse host:port address %q: missing port", addr)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("address %q has non-numeric port %q", addr, port)
	}

	return fmt.Sprintf("RIDE_INIT=SERVE:*:%s %s +s -q", port, shellEscape(executable)), nil
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`!&|;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
