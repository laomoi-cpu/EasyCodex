package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"easycodex-agent/internal/config"
	"easycodex-agent/internal/netinfo"
	"easycodex-agent/internal/server"
	"easycodex-agent/internal/wezterm"
)

func main() {
	configPath := flag.String("config", "", "config file path")
	listenOverride := flag.String("listen", "", "override listen address, for example 127.0.0.1:8765")
	tokenOverride := flag.String("token", "", "override API token")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, found, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if *listenOverride != "" {
		cfg.Listen = *listenOverride
	}
	if *tokenOverride != "" {
		cfg.Token = *tokenOverride
	}
	config.Normalize(&cfg)
	if err := config.Validate(cfg); err != nil {
		logger.Error("invalid config", "error", err)
		os.Exit(1)
	}
	displayConfigPath := *configPath
	if displayConfigPath == "" {
		displayConfigPath = filepath.Join(cfg.Root, "agent", "config.json")
	}
	if initialized, err := initializeMissingConfig(displayConfigPath, cfg, found); err != nil {
		logger.Error("failed to initialize config", "config", displayConfigPath, "error", err)
		os.Exit(1)
	} else if initialized {
		logger.Info("initialized config with generated token", "config", displayConfigPath)
		found = true
	}
	if *tokenOverride == "" {
		if changed, err := regenerateStartupToken(displayConfigPath, &cfg); err != nil {
			logger.Error("failed to regenerate startup token", "error", err)
			os.Exit(1)
		} else if changed {
			logger.Info("regenerated startup token", "config", displayConfigPath)
		}
	}

	cli := wezterm.CLI{Root: cfg.Root, Timeout: cfg.CommandTimeout()}
	tracker := &trackedWezTerm{cli: cli, logger: logger}
	app, err := server.NewWithConfigPath(cfg, displayConfigPath, tracker, logger)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Println("EasyCodex Agent started")
	if found {
		fmt.Printf("Config: %s\n", displayConfigPath)
	} else {
		fmt.Printf("Config: %s not found, using defaults\n", displayConfigPath)
	}
	network := netinfo.Inspect(cfg.Listen)
	fmt.Printf("Local: %s\n", network.LocalURL)
	if network.LANEnabled {
		if len(network.LANURLs) == 0 {
			fmt.Println("LAN:   enabled, but no LAN IPv4 address was detected")
		}
		for _, url := range network.LANURLs {
			fmt.Printf("LAN:   %s\n", url)
		}
	} else {
		fmt.Println("LAN:   disabled; set listen to 0.0.0.0:8765 to allow phone access")
	}
	fmt.Printf("Root:  %s\n", cfg.Root)
	fmt.Printf("Token: %s\n", cfg.Token)
	for _, instance := range cfg.Instances {
		fmt.Printf("Instance: %s (%s) class=%s\n", instance.ID, instance.Name, instance.Class)
	}
	startTrayHelper(logger, cfg, displayConfigPath)
	autoLaunchInstances(context.Background(), logger, tracker, cfg)
	defer cleanupLaunchedGUI(logger, tracker, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server exited unexpectedly", "error", err)
			cleanupLaunchedGUI(logger, tracker, cfg)
			os.Exit(1)
		}
	case <-stop:
		logger.Info("stopping server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("failed to stop server", "error", err)
			os.Exit(1)
		}
	}
}

func initializeMissingConfig(configPath string, cfg config.Config, found bool) (bool, error) {
	if found {
		return false, nil
	}
	if err := config.Save(configPath, cfg); err != nil {
		return false, err
	}
	return true, nil
}

func regenerateStartupToken(configPath string, cfg *config.Config) (bool, error) {
	if !cfg.RegenerateTokenOnStart {
		return false, nil
	}
	token, err := config.GenerateToken()
	if err != nil {
		return false, err
	}
	cfg.Token = token
	return true, config.Save(configPath, *cfg)
}

func startTrayHelper(logger *slog.Logger, cfg config.Config, configPath string) {
	if runtime.GOOS != "windows" {
		return
	}
	scriptPath := filepath.Join(cfg.Root, "agent", "tray.ps1")
	if _, err := os.Stat(scriptPath); err != nil {
		logger.Warn("tray helper not found", "path", scriptPath, "error", err)
		return
	}
	cleanupExistingTrayHelpers(logger, scriptPath)
	pairingURL := netinfo.Inspect(cfg.Listen).LocalURL + "/pairing"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath, "-PairingUrl", pairingURL, "-ConfigPath", configPath, "-AgentPid", strconv.Itoa(os.Getpid()))
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		logger.Warn("failed to start tray helper", "error", err)
		return
	}
	_ = cmd.Process.Release()
}

func cleanupExistingTrayHelpers(logger *slog.Logger, scriptPath string) {
	if runtime.GOOS != "windows" {
		return
	}
	escapedPath := strings.ReplaceAll(scriptPath, "'", "''")
	script := fmt.Sprintf(`$target = '%s'; Get-CimInstance Win32_Process -Filter "Name = 'powershell.exe'" | Where-Object { $_.ProcessId -ne $PID -and $_.CommandLine -like "*$target*" } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`, escapedPath)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err := cmd.Run(); err != nil {
		logger.Warn("failed to cleanup existing tray helpers", "error", err)
	}
}

type trackedWezTerm struct {
	cli      wezterm.CLI
	logger   *slog.Logger
	mu       sync.Mutex
	launched []launchedGUI
}

type launchedGUI struct {
	PID   int
	Class string
}

type launchListClient interface {
	Launch(ctx context.Context, class string) (int, error)
	List(ctx context.Context, class string) (json.RawMessage, error)
}

var hasGUIProcess = weztermGUIProcessExists

func (tracked *trackedWezTerm) Launch(ctx context.Context, class string) (int, error) {
	pid, err := tracked.cli.Launch(ctx, class)
	if err != nil {
		return 0, err
	}
	tracked.mu.Lock()
	tracked.launched = append(tracked.launched, launchedGUI{PID: pid, Class: class})
	tracked.mu.Unlock()
	return pid, nil
}

func (tracked *trackedWezTerm) List(ctx context.Context, class string) (json.RawMessage, error) {
	return tracked.cli.List(ctx, class)
}

func (tracked *trackedWezTerm) GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error) {
	return tracked.cli.GetText(ctx, class, paneID, lines, escapes)
}

func (tracked *trackedWezTerm) SendText(ctx context.Context, class, paneID, text string, noPaste bool) error {
	return tracked.cli.SendText(ctx, class, paneID, text, noPaste)
}

func (tracked *trackedWezTerm) KillPane(ctx context.Context, class, paneID string) error {
	return tracked.cli.KillPane(ctx, class, paneID)
}

func (tracked *trackedWezTerm) Spawn(ctx context.Context, class, paneID, cwd string, newWindow bool, command []string) (string, error) {
	return tracked.cli.Spawn(ctx, class, paneID, cwd, newWindow, command)
}

func (tracked *trackedWezTerm) launchedGUI() []launchedGUI {
	tracked.mu.Lock()
	defer tracked.mu.Unlock()
	items := make([]launchedGUI, len(tracked.launched))
	copy(items, tracked.launched)
	return items
}

func autoLaunchInstances(ctx context.Context, logger *slog.Logger, weztermClient launchListClient, cfg config.Config) {
	instances := make(map[string]config.Instance, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		instances[instance.ID] = instance
	}

	for _, id := range cfg.AutoLaunch {
		if id == "" {
			continue
		}
		instance, ok := instances[id]
		if !ok {
			logger.Warn("auto launch skipped unknown instance", "instance", id)
			continue
		}

		exists, err := hasGUIProcess(instance.Class)
		if err != nil {
			logger.Warn("auto launch gui check failed", "instance", id, "class", instance.Class, "error", err)
		} else if exists {
			logger.Info("auto launch skipped existing gui", "instance", id, "class", instance.Class)
			continue
		}

		pid, err := weztermClient.Launch(ctx, instance.Class)
		if err != nil {
			logger.Error("auto launch failed", "instance", id, "class", instance.Class, "error", err)
			continue
		}
		logger.Info("auto launched instance", "instance", id, "class", instance.Class, "pid", pid)
	}
}

func weztermGUIProcessExists(class string) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	escapedClass := strings.ReplaceAll(class, "'", "''")
	script := fmt.Sprintf(`$class = '%s'; $pattern = '(^|\s)--class\s+' + [regex]::Escape($class) + '($|\s)'; $items = Get-CimInstance Win32_Process -Filter "Name = 'wezterm-gui.exe'" | Where-Object { $_.CommandLine -match $pattern }; if ($items) { 'true' } else { 'false' }`, escapedClass)
	out, err := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func instanceHasSessions(ctx context.Context, weztermClient launchListClient, class string) (bool, error) {
	data, err := weztermClient.List(ctx, class)
	if err != nil {
		return false, err
	}
	var sessions []any
	if err := json.Unmarshal(data, &sessions); err != nil {
		return false, err
	}
	return len(sessions) > 0, nil
}

func cleanupLaunchedGUI(logger *slog.Logger, tracker *trackedWezTerm, cfg config.Config) {
	if !cfg.CloseLaunchedGUIOnExit {
		return
	}
	for _, item := range tracker.launchedGUI() {
		process, err := os.FindProcess(item.PID)
		if err != nil {
			logger.Warn("failed to find launched gui process", "pid", item.PID, "class", item.Class, "error", err)
			continue
		}
		if err := process.Kill(); err != nil {
			logger.Warn("failed to close launched gui process", "pid", item.PID, "class", item.Class, "error", err)
			continue
		}
		logger.Info("closed launched gui process", "pid", item.PID, "class", item.Class)
	}
}
