package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"easyterm-agent/internal/config"
	"easyterm-agent/internal/server"
	"easyterm-agent/internal/wezterm"
)

func main() {
	configPath := flag.String("config", filepath.Join("agent", "config.json"), "config file path")
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
	app, err := server.New(cfg, cli, logger)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Println("EasyTerm Agent started")
	if found {
		fmt.Printf("Config: %s\n", *configPath)
	} else {
		fmt.Printf("Config: %s not found, using defaults\n", *configPath)
	}
	fmt.Printf("Local: http://%s\n", cfg.Listen)
	if host, _, err := net.SplitHostPort(cfg.Listen); err == nil && (host == "0.0.0.0" || host == "") {
		printLANAddresses(cfg.Listen)
	}
	fmt.Printf("Root:  %s\n", cfg.Root)
	fmt.Printf("Token: %s\n", cfg.Token)
	for _, instance := range cfg.Instances {
		fmt.Printf("Instance: %s (%s) class=%s\n", instance.ID, instance.Name, instance.Class)
	}
	autoLaunchInstances(context.Background(), logger, cli, cfg)

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

func autoLaunchInstances(ctx context.Context, logger *slog.Logger, cli wezterm.CLI, cfg config.Config) {
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
		if err := cli.Launch(ctx, instance.Class); err != nil {
			logger.Error("auto launch failed", "instance", id, "class", instance.Class, "error", err)
			continue
		}
		logger.Info("auto launched instance", "instance", id, "class", instance.Class)
	}
}

func printLANAddresses(listen string) {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}
		fmt.Printf("LAN:   http://%s:%s\n", ip.String(), port)
	}
}
