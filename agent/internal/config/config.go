package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultListen         = "127.0.0.1:8765"
	DefaultCommandTimeout = 5 * time.Second
)

type Config struct {
	Listen                 string         `json:"listen"`
	Root                   string         `json:"root"`
	Token                  string         `json:"token"`
	CommandTimeoutSeconds  int            `json:"commandTimeoutSeconds"`
	AutoLaunch             []string       `json:"autoLaunch"`
	CloseLaunchedGUIOnExit bool           `json:"closeLaunchedGuiOnExit"`
	Instances              []Instance     `json:"instances"`
	MobileDefaults         MobileDefaults `json:"mobileDefaults"`
}

type Instance struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Class string `json:"class"`
}

type MobileDefaults struct {
	InstanceID string   `json:"instanceId"`
	CWD        string   `json:"cwd"`
	Command    []string `json:"command"`
}

func Load(path string) (Config, bool, error) {
	if path == "" {
		path = DefaultPath()
	}

	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if cfg.Token == "" {
				token, tokenErr := GenerateToken()
				if tokenErr != nil {
					return Config{}, false, tokenErr
				}
				cfg.Token = token
			}
			return cfg, false, nil
		}
		return Config{}, false, err
	}

	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, err
	}
	Normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, true, err
	}
	if cfg.Token == "" {
		token, tokenErr := GenerateToken()
		if tokenErr != nil {
			return Config{}, true, tokenErr
		}
		cfg.Token = token
	}
	return cfg, true, nil
}

func DefaultPath() string {
	root := os.Getenv("EASYCODEX_ROOT")
	if root == "" {
		root = inferRoot()
	}
	return filepath.Join(root, "agent", "config.json")
}

func Save(path string, cfg Config) error {
	if path == "" {
		path = DefaultPath()
	}
	Normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func Defaults() Config {
	root := os.Getenv("EASYCODEX_ROOT")
	if root == "" {
		root = inferRoot()
	}

	cfg := Config{
		Listen:                DefaultListen,
		Root:                  root,
		CommandTimeoutSeconds: int(DefaultCommandTimeout / time.Second),
		AutoLaunch:            []string{"main"},
		Instances: []Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
		MobileDefaults: MobileDefaults{
			InstanceID: "main",
			CWD:        `D:\mgame`,
			Command:    defaultMobileCommand(),
		},
	}
	Normalize(&cfg)
	return cfg
}

func Normalize(cfg *Config) {
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.Root = strings.TrimSpace(cfg.Root)
	if cfg.Root == "" {
		cfg.Root = inferRoot()
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	for i := range cfg.AutoLaunch {
		cfg.AutoLaunch[i] = strings.TrimSpace(cfg.AutoLaunch[i])
	}
	for i := range cfg.Instances {
		cfg.Instances[i].ID = strings.TrimSpace(cfg.Instances[i].ID)
		cfg.Instances[i].Name = strings.TrimSpace(cfg.Instances[i].Name)
		cfg.Instances[i].Class = strings.TrimSpace(cfg.Instances[i].Class)
	}
	cfg.MobileDefaults.InstanceID = strings.TrimSpace(cfg.MobileDefaults.InstanceID)
	cfg.MobileDefaults.CWD = strings.TrimSpace(cfg.MobileDefaults.CWD)
	if cfg.MobileDefaults.CWD == "" {
		cfg.MobileDefaults.CWD = `D:\mgame`
	}
	for i := range cfg.MobileDefaults.Command {
		cfg.MobileDefaults.Command[i] = strings.TrimSpace(cfg.MobileDefaults.Command[i])
	}
	if len(cfg.MobileDefaults.Command) == 0 {
		cfg.MobileDefaults.Command = defaultMobileCommand()
	}
	if len(cfg.Instances) > 0 {
		defaultInstanceFound := cfg.MobileDefaults.InstanceID == ""
		for _, instance := range cfg.Instances {
			if instance.ID == cfg.MobileDefaults.InstanceID {
				defaultInstanceFound = true
				break
			}
		}
		if !defaultInstanceFound {
			cfg.MobileDefaults.InstanceID = cfg.Instances[0].ID
		}
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if cfg.CommandTimeoutSeconds <= 0 {
		cfg.CommandTimeoutSeconds = int(DefaultCommandTimeout / time.Second)
	}
}

func Validate(cfg Config) error {
	if cfg.Root == "" {
		return errors.New("root is required")
	}
	if len(cfg.Instances) == 0 {
		return errors.New("at least one instance is required")
	}

	seen := make(map[string]struct{}, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		if instance.ID == "" {
			return errors.New("instance id is required")
		}
		if instance.Class == "" {
			return fmt.Errorf("instance %q class is required", instance.ID)
		}
		if _, ok := seen[instance.ID]; ok {
			return fmt.Errorf("duplicate instance id %q", instance.ID)
		}
		seen[instance.ID] = struct{}{}
	}
	if cfg.MobileDefaults.InstanceID != "" {
		if _, ok := seen[cfg.MobileDefaults.InstanceID]; !ok {
			return fmt.Errorf("mobile default instance %q is not configured", cfg.MobileDefaults.InstanceID)
		}
	}
	if len(cfg.MobileDefaults.Command) == 0 {
		return errors.New("mobile default command is required")
	}
	for _, id := range cfg.AutoLaunch {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("auto launch instance %q is not configured", id)
		}
	}
	return nil
}

func defaultMobileCommand() []string {
	return []string{"cmd.exe", "/k", `cd /d D:\mgame && codex --dangerously-bypass-approvals-and-sandbox`}
}
func (cfg Config) CommandTimeout() time.Duration {
	if cfg.CommandTimeoutSeconds <= 0 {
		return DefaultCommandTimeout
	}
	return time.Duration(cfg.CommandTimeoutSeconds) * time.Second
}

func GenerateToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func inferRoot() string {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, start := range candidates {
		dir := start
		for {
			if _, err := os.Stat(filepath.Join(dir, "bin", "wezterm.exe")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "."
}
