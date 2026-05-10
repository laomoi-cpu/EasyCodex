package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingUsesDefaultsAndRuntimeToken(t *testing.T) {
	cfg, found, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if found {
		t.Fatalf("expected missing config")
	}
	if cfg.Listen != DefaultListen {
		t.Fatalf("listen = %q", cfg.Listen)
	}
	if cfg.Token == "" {
		t.Fatalf("expected generated token")
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0].Class != "easyterm" {
		t.Fatalf("unexpected instances: %#v", cfg.Instances)
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"listen": "0.0.0.0:9000",
		"root": "D:\\EasyTerm",
		"token": "secret",
		"commandTimeoutSeconds": 9,
		"instances": [{"id": "work", "name": "工作", "class": "easyterm-work"}]
	}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, found, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected config file")
	}
	if cfg.Token != "secret" || cfg.CommandTimeoutSeconds != 9 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Instances[0].ID != "work" {
		t.Fatalf("unexpected instance: %#v", cfg.Instances[0])
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	cfg := Defaults()
	cfg.Instances = []Instance{
		{ID: "main", Class: "easyterm"},
		{ID: "main", Class: "easyterm-2"},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}
