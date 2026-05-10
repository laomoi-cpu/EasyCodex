package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

	cli := wezterm.CLI{Root: cfg.Root, Timeout: cfg.CommandTimeout()}
	tracker := &trackedWezTerm{cli: cli, logger: logger}
	app, err := server.New(cfg, tracker, logger)
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
	displayConfigPath := *configPath
	if displayConfigPath == "" {
		displayConfigPath = filepath.Join(cfg.Root, "agent", "config.json")
	}
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

		pid, err := weztermClient.Launch(ctx, instance.Class)
		if err != nil {
			logger.Error("auto launch failed", "instance", id, "class", instance.Class, "error", err)
			continue
		}
		logger.Info("auto launched instance", "instance", id, "class", instance.Class, "pid", pid)
	}
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
