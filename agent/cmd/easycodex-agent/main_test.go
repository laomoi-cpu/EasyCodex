package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"easycodex-agent/internal/config"
)

type fakeLaunchListClient struct {
	listPayload json.RawMessage
	listErr     error
	launches    []string
}

func TestRegenerateStartupTokenSavesNewToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Defaults()
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "old-token"
	cfg.RegenerateTokenOnStart = true

	changed, err := regenerateStartupToken(path, &cfg)
	if err != nil {
		t.Fatalf("regenerateStartupToken returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected startup token to be regenerated")
	}
	if cfg.Token == "" || cfg.Token == "old-token" {
		t.Fatalf("unexpected token after regenerate: %q", cfg.Token)
	}
	loaded, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found || loaded.Token != cfg.Token || !loaded.RegenerateTokenOnStart {
		t.Fatalf("unexpected saved config: found=%v cfg=%#v", found, loaded)
	}
}

func TestRegenerateStartupTokenSkipsWhenDisabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Token = "stable-token"

	changed, err := regenerateStartupToken(filepath.Join(t.TempDir(), "config.json"), &cfg)
	if err != nil {
		t.Fatalf("regenerateStartupToken returned error: %v", err)
	}
	if changed || cfg.Token != "stable-token" {
		t.Fatalf("unexpected regenerate result: changed=%v token=%q", changed, cfg.Token)
	}
}

func (fake *fakeLaunchListClient) Launch(ctx context.Context, class string) (int, error) {
	fake.launches = append(fake.launches, class)
	return 1000 + len(fake.launches), nil
}

func (fake *fakeLaunchListClient) List(ctx context.Context, class string) (json.RawMessage, error) {
	if fake.listErr != nil {
		return nil, fake.listErr
	}
	return fake.listPayload, nil
}

func TestInstanceHasSessions(t *testing.T) {
	client := &fakeLaunchListClient{listPayload: json.RawMessage(`[{"pane_id":1}]`)}
	exists, err := instanceHasSessions(context.Background(), client, "easycodex")
	if err != nil {
		t.Fatalf("instanceHasSessions returned error: %v", err)
	}
	if !exists {
		t.Fatalf("expected existing sessions")
	}
}

func TestAutoLaunchSkipsWhenGUIExists(t *testing.T) {
	withGUIProcessChecker(t, func(class string) (bool, error) {
		return true, nil
	})
	client := &fakeLaunchListClient{listPayload: json.RawMessage(`[{"pane_id":1}]`)}
	cfg := config.Config{
		Root:       `D:\EasyCodex`,
		AutoLaunch: []string{"main"},
		Instances:  []config.Instance{{ID: "main", Name: "main", Class: "easycodex"}},
	}

	autoLaunchInstances(context.Background(), discardLogger(), client, cfg)

	if len(client.launches) != 0 {
		t.Fatalf("launches = %#v", client.launches)
	}
}

func TestAutoLaunchLaunchesWhenGUIMissingEvenWhenSessionExists(t *testing.T) {
	withGUIProcessChecker(t, func(class string) (bool, error) {
		return false, nil
	})
	client := &fakeLaunchListClient{listPayload: json.RawMessage(`[{"pane_id":1}]`)}
	cfg := config.Config{
		Root:       `D:\EasyCodex`,
		AutoLaunch: []string{"main"},
		Instances:  []config.Instance{{ID: "main", Name: "main", Class: "easycodex"}},
	}

	autoLaunchInstances(context.Background(), discardLogger(), client, cfg)

	if len(client.launches) != 1 || client.launches[0] != "easycodex" {
		t.Fatalf("launches = %#v", client.launches)
	}
}

func TestAutoLaunchLaunchesWhenListIsEmpty(t *testing.T) {
	withGUIProcessChecker(t, func(class string) (bool, error) {
		return false, nil
	})
	client := &fakeLaunchListClient{listPayload: json.RawMessage(`[]`)}
	cfg := config.Config{
		Root:       `D:\EasyCodex`,
		AutoLaunch: []string{"main"},
		Instances:  []config.Instance{{ID: "main", Name: "main", Class: "easycodex"}},
	}

	autoLaunchInstances(context.Background(), discardLogger(), client, cfg)

	if len(client.launches) != 1 || client.launches[0] != "easycodex" {
		t.Fatalf("launches = %#v", client.launches)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func withGUIProcessChecker(t *testing.T, checker func(string) (bool, error)) {
	t.Helper()
	previous := hasGUIProcess
	hasGUIProcess = checker
	t.Cleanup(func() {
		hasGUIProcess = previous
	})
}
