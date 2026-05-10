package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(cfg.AutoLaunch) != 1 || cfg.AutoLaunch[0] != "main" {
		t.Fatalf("unexpected auto launch: %#v", cfg.AutoLaunch)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0].Class != "easycodex" {
		t.Fatalf("unexpected instances: %#v", cfg.Instances)
	}
}

func TestLoadDefaultPathUsesInferredRoot(t *testing.T) {
	t.Setenv("EASYCODEX_ROOT", t.TempDir())

	cfg, found, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if found {
		t.Fatalf("expected missing config")
	}
	if cfg.Root == "" {
		t.Fatalf("expected inferred root")
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"listen": "0.0.0.0:9000",
		"root": "D:\\EasyCodex",
		"token": "secret",
		"commandTimeoutSeconds": 9,
		"autoLaunch": [],
		"instances": [{"id": "work", "name": "work", "class": "easycodex-work"}]
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
	if len(cfg.AutoLaunch) != 0 {
		t.Fatalf("unexpected auto launch: %#v", cfg.AutoLaunch)
	}
	if cfg.Instances[0].ID != "work" {
		t.Fatalf("unexpected instance: %#v", cfg.Instances[0])
	}
}

func TestLoadConfigFileWithUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{
		"listen": "127.0.0.1:8765",
		"root": "D:\\EasyCodex",
		"token": "secret",
		"instances": [{"id": "main", "name": "main", "class": "easycodex"}]
	}`)...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, found, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found || cfg.Token != "secret" {
		t.Fatalf("unexpected config: found=%v cfg=%#v", found, cfg)
	}
}

func TestSaveWritesConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Defaults()
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "saved-token"
	cfg.RegenerateTokenOnStart = true
	cfg.Listen = "0.0.0.0:8765"
	cfg.MobileDefaults.CWD = `D:\mgame`
	cfg.MobileDefaults.Command = []string{"cmd.exe", "/k", `cd /d D:\mgame && codex`}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `&& codex`) {
		t.Fatalf("expected readable command in saved config: %s", data)
	}
	if !strings.Contains(string(data), `"regenerateTokenOnStart": true`) {
		t.Fatalf("expected token startup option in saved config: %s", data)
	}
	loaded, found, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found || loaded.Token != "saved-token" || !loaded.RegenerateTokenOnStart || loaded.Listen != "0.0.0.0:8765" {
		t.Fatalf("unexpected loaded config: found=%v cfg=%#v", found, loaded)
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	cfg := Defaults()
	cfg.Instances = []Instance{
		{ID: "main", Class: "easycodex"},
		{ID: "main", Class: "easycodex-2"},
	}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}

func TestValidateRejectsUnknownAutoLaunch(t *testing.T) {
	cfg := Defaults()
	cfg.AutoLaunch = []string{"missing"}
	if err := Validate(cfg); err == nil {
		t.Fatalf("expected unknown auto launch error")
	}
}
