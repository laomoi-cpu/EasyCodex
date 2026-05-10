package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"easycodex-agent/internal/config"
)

type fakeLaunchListClient struct {
	listPayload json.RawMessage
	listErr     error
	launches    []string
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

func TestAutoLaunchSkipsExistingInstance(t *testing.T) {
	client := &fakeLaunchListClient{listPayload: json.RawMessage(`[{"pane_id":1}]`)}
	cfg := config.Config{
		Root:       `D:\EasyCodex`,
		AutoLaunch: []string{"main"},
		Instances:  []config.Instance{{ID: "main", Name: "main", Class: "easycodex"}},
	}

	autoLaunchInstances(context.Background(), discardLogger(), client, cfg)

	if len(client.launches) != 0 {
		t.Fatalf("unexpected launches: %#v", client.launches)
	}
}

func TestAutoLaunchLaunchesWhenListIsEmpty(t *testing.T) {
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
