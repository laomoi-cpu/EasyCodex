package wezterm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CLI struct {
	Root    string
	Timeout time.Duration
}

type CommandError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (err *CommandError) Error() string {
	if err.Stderr != "" {
		return fmt.Sprintf("wezterm cli failed with exit code %d: %s", err.ExitCode, err.Stderr)
	}
	return fmt.Sprintf("wezterm cli failed with exit code %d", err.ExitCode)
}

func (cli CLI) List(ctx context.Context, class string) (json.RawMessage, error) {
	stdout, err := cli.run(ctx, class, nil, "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(stdout), nil
}

func (cli CLI) GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error) {
	args := []string{"get-text", "--pane-id", paneID}
	if lines > 0 {
		args = append(args, "--start-line", "-"+strconv.Itoa(lines))
	}
	if escapes {
		args = append(args, "--escapes")
	}
	stdout, err := cli.run(ctx, class, nil, args...)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

func (cli CLI) SendText(ctx context.Context, class, paneID, text string, noPaste bool) error {
	args := []string{"send-text", "--pane-id", paneID}
	if noPaste {
		args = append(args, "--no-paste")
	}
	_, err := cli.run(ctx, class, strings.NewReader(text), args...)
	return err
}

func (cli CLI) Spawn(ctx context.Context, class, cwd string, newWindow bool, command []string) (string, error) {
	args := []string{"spawn"}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	if newWindow {
		args = append(args, "--new-window")
	}
	args = append(args, command...)
	stdout, err := cli.run(ctx, class, nil, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (cli CLI) Launch(ctx context.Context, class string) (int, error) {
	if class == "" {
		return 0, errors.New("class is required")
	}
	root, err := filepath.Abs(cli.Root)
	if err != nil {
		return 0, err
	}
	guiPath := filepath.Join(root, "bin", "wezterm-gui.exe")
	if _, err := os.Stat(guiPath); err != nil {
		return 0, fmt.Errorf("wezterm gui executable not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, guiPath, "start", "--class", class)
	cmd.Dir = root
	cmd.Env = append(
		os.Environ(),
		"WEZTERM_CONFIG_FILE="+filepath.Join(root, "wezterm-config", "wezterm.lua"),
		"EASYCODEX_ROOT="+root,
	)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}

func (cli CLI) run(ctx context.Context, class string, stdin *strings.Reader, args ...string) ([]byte, error) {
	if class == "" {
		return nil, errors.New("class is required")
	}
	root, err := filepath.Abs(cli.Root)
	if err != nil {
		return nil, err
	}
	weztermPath := filepath.Join(root, "bin", "wezterm.exe")
	if _, err := os.Stat(weztermPath); err != nil {
		return nil, fmt.Errorf("wezterm executable not found: %w", err)
	}

	timeout := cli.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	allArgs := append([]string{"cli", "--class", class}, args...)
	cmd := exec.CommandContext(cmdCtx, weztermPath, allArgs...)
	cmd.Dir = muxDir(root)
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if cmdCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("wezterm cli timed out after %s", timeout)
	}
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return nil, &CommandError{
			Args:     allArgs,
			ExitCode: exitCode,
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}
	return stdout.Bytes(), nil
}

func muxDir(fallback string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return fallback
	}
	dir := filepath.Join(home, ".local", "share", "wezterm")
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	return fallback
}
