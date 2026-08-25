package transport

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func Receive(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("receive_command is not configured")
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("receive command failed: %w", err)
	}
	return string(out), nil
}

func Send(ctx context.Context, command string, raw []byte) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("send_command is not configured")
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("send command failed: %w", err)
	}
	return string(out), nil
}
