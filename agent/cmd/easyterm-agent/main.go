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
	configPath := flag.String("config", filepath.Join("agent", "config.json"), "配置文件路径")
	listenOverride := flag.String("listen", "", "覆盖监听地址，例如 127.0.0.1:8765")
	tokenOverride := flag.String("token", "", "覆盖 API token")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, found, err := config.Load(*configPath)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
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
		logger.Error("配置无效", "error", err)
		os.Exit(1)
	}

	cli := wezterm.CLI{Root: cfg.Root, Timeout: cfg.CommandTimeout()}
	app, err := server.New(cfg, cli, logger)
	if err != nil {
		logger.Error("创建服务失败", "error", err)
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("服务异常退出", "error", err)
			os.Exit(1)
		}
	case <-stop:
		logger.Info("正在停止服务")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("停止服务失败", "error", err)
			os.Exit(1)
		}
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
