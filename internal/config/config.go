package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent AgentConfig `yaml:"agent"`
	Rails RailsConfig `yaml:"rails"`
}

type AgentConfig struct {
	WatchPaths          []string `yaml:"watch_paths"`
	ScanIntervalSeconds int      `yaml:"scan_interval_seconds"`
	StateFile           string   `yaml:"state_file"`
	SpoolDir            string   `yaml:"spool_dir"`
	MaxRetries          int      `yaml:"max_retries"`
}

func (a AgentConfig) ScanInterval() time.Duration {
	if a.ScanIntervalSeconds <= 0 {
		return 10 * time.Second
	}
	return time.Duration(a.ScanIntervalSeconds) * time.Second
}

type RailsConfig struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	UploadPath string `yaml:"upload_path"`
	HealthPath string `yaml:"health_path"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}

	if err := cfg.setDefaults(path); err != nil {
		return Config{}, err
	}

	if cfg.Rails.BaseURL == "" {
		return Config{}, fmt.Errorf("rails.base_url is required")
	}
	if cfg.Rails.APIKey == "" {
		return Config{}, fmt.Errorf("rails.api_key is required")
	}

	return cfg, nil
}

func (c *Config) setDefaults(configPath string) error {
	if len(c.Agent.WatchPaths) == 0 {
		c.Agent.WatchPaths = defaultTelemetryPaths()
	}

	if c.Agent.ScanIntervalSeconds <= 0 {
		c.Agent.ScanIntervalSeconds = 10
	}
	if c.Agent.MaxRetries <= 0 {
		c.Agent.MaxRetries = 8
	}

	baseDir := filepath.Dir(configPath)
	if c.Agent.StateFile == "" {
		c.Agent.StateFile = filepath.Join(baseDir, "agent-state.json")
	}
	if c.Agent.SpoolDir == "" {
		c.Agent.SpoolDir = filepath.Join(baseDir, "spool")
	}
	if c.Rails.UploadPath == "" {
		c.Rails.UploadPath = "/api/v1/telemetry_uploads"
	}
	if c.Rails.HealthPath == "" {
		c.Rails.HealthPath = "/up"
	}

	return nil
}

func defaultTelemetryPaths() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return []string{"./telemetry"}
	}

	switch runtime.GOOS {
	case "windows":
		paths := []string{
			filepath.Join(homeDir, "Documents", "iRacing", "telemetry"),
		}
		oneDrive := os.Getenv("OneDrive")
		if oneDrive != "" {
			paths = append(paths, filepath.Join(oneDrive, "Documents", "iRacing", "telemetry"))
		}
		return uniquePaths(paths)
	default:
		return []string{filepath.Join(homeDir, "Documents", "iRacing", "telemetry")}
	}
}

func uniquePaths(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, path := range input {
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
