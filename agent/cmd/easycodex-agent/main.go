package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
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
	"easycodex-agent/internal/winproc"
)

var version = "dev"

const releaseDownloadsURL = "https://github.com/laomoi-cpu/EasyCodex/releases/latest"

func main() {
	configPath := flag.String("config", "", "config file path")
	listenOverride := flag.String("listen", "", "override listen address, for example 127.0.0.1:8765")
	tokenOverride := flag.String("token", "", "override API token")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, found, err := config.Load(*configPath)
	if err != nil {
		fatalStartup(logger, "Failed to load config", err)
	}
	if *listenOverride != "" {
		cfg.Listen = *listenOverride
	}
	if *tokenOverride != "" {
		cfg.Token = *tokenOverride
	}
	config.Normalize(&cfg)
	if err := config.Validate(cfg); err != nil {
		fatalStartup(logger, "Invalid config", err)
	}
	logger, logFile := newLogger(cfg.Root)
	if logFile != nil {
		defer logFile.Close()
	}
	if err := validateInstallRoot(cfg.Root); err != nil {
		logger.Error("incomplete EasyCodex package", "root", cfg.Root, "error", err)
		openURL(releaseDownloadsURL, logger)
		fatalStartup(logger, "Incomplete EasyCodex package", err)
	}
	displayConfigPath := *configPath
	if displayConfigPath == "" {
		displayConfigPath = filepath.Join(cfg.Root, "agent", "config.json")
	}
	if initialized, err := initializeMissingConfig(displayConfigPath, cfg, found); err != nil {
		fatalStartup(logger, "Failed to initialize config", err)
	} else if initialized {
		logger.Info("initialized config with generated token", "config", displayConfigPath)
		found = true
	}
	if *tokenOverride == "" {
		if changed, err := regenerateStartupToken(displayConfigPath, &cfg); err != nil {
			fatalStartup(logger, "Failed to regenerate startup token", err)
		} else if changed {
			logger.Info("regenerated startup token", "config", displayConfigPath)
		}
	}

	cli := wezterm.CLI{Root: cfg.Root, Timeout: cfg.CommandTimeout()}
	tracker := &trackedWezTerm{cli: cli, logger: logger}
	server.AppVersion = version
	app, err := server.NewWithConfigPath(cfg, displayConfigPath, tracker, logger)
	if err != nil {
		fatalStartup(logger, "Failed to create server", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		if openExistingAgent(cfg.Listen, logger) {
			return
		}
		fatalStartup(logger, "Failed to start EasyCodex Agent", fmt.Errorf("listen %s: %w", cfg.Listen, err))
	}

	fmt.Println("EasyCodex Agent started")
	fmt.Printf("Version: %s\n", version)
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
	openURL(network.LocalURL+"/settings", logger)
	autoLaunchInstances(context.Background(), logger, tracker, cfg)
	defer cleanupLaunchedGUI(logger, tracker, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server exited unexpectedly", "error", err)
			cleanupLaunchedGUI(logger, tracker, cfg)
			fatalStartup(logger, "EasyCodex Agent exited unexpectedly", err)
		}
	case <-stop:
		logger.Info("stopping server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			fatalStartup(logger, "Failed to stop EasyCodex Agent", err)
		}
	}
}

func fatalStartup(logger *slog.Logger, title string, err error) {
	if logger != nil {
		logger.Error(title, "error", err)
	}
	showStartupError(title, err)
	os.Exit(1)
}

func validateInstallRoot(root string) error {
	required := []string{
		filepath.Join(root, "bin", "wezterm.exe"),
		filepath.Join(root, "bin", "wezterm-gui.exe"),
		filepath.Join(root, "agent", "tray.ps1"),
		filepath.Join(root, "wezterm-config", "wezterm.lua"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing required file %s: %w", path, err)
		}
	}
	return nil
}

func newLogger(root string) (*slog.Logger, *os.File) {
	logPath := filepath.Join(root, ".logs", "easycodex-agent.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
	}
	writer := io.MultiWriter(os.Stdout, file)
	return slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})), file
}

func openExistingAgent(listen string, logger *slog.Logger) bool {
	url := netinfo.Inspect(listen).LocalURL
	client := http.Client{Timeout: 1200 * time.Millisecond}
	res, err := client.Get(url + "/api/health")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Service string `json:"service"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil || !payload.OK || payload.Data.Service != "easycodex-agent" {
		return false
	}
	settingsURL := url + "/settings"
	logger.Info("agent already running, opening existing console", "url", settingsURL)
	openURL(settingsURL, logger)
	return true
}

func openURL(url string, logger *slog.Logger) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
		winproc.HideWindow(cmd)
		if err := cmd.Start(); err != nil {
			logger.Warn("failed to open url", "url", url, "error", err)
		}
		return
	}
	logger.Info("open this URL", "url", url)
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
	winproc.HideWindow(cmd)
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
	winproc.HideWindow(cmd)
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	winproc.HideWindow(cmd)
	out, err := cmd.Output()
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
