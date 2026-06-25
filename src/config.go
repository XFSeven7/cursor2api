// 读取并校验 config.json，填充默认值。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 服务运行配置。
type Config struct {
	Port             int    `json:"port"`
	APIKey           string `json:"apiKey"`
	AgentPath        string `json:"agentPath"`
	Workspace        string `json:"workspace"`
	DefaultModel     string `json:"defaultModel"`
	AgentMode        string `json:"agentMode"`
	SessionTTLMs     int64  `json:"sessionTtlMs"`
	RequestTimeoutMs int64  `json:"requestTimeoutMs"`
	LoginCacheMs     int64  `json:"loginCacheMs"`
}

// LoadConfig 从文件加载配置并应用默认值。
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 3010
	}
	if cfg.APIKey == "" {
		cfg.APIKey = "sk-cursor2api"
	}
	if cfg.AgentPath == "" {
		cfg.AgentPath = "agent"
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = autoModelID
	}
	if cfg.DefaultModel != autoModelID {
		cfg.DefaultModel = autoModelID
	}
	if cfg.AgentMode == "" {
		cfg.AgentMode = "ask"
	}
	if cfg.SessionTTLMs == 0 {
		cfg.SessionTTLMs = 3_600_000
	}
	if cfg.RequestTimeoutMs == 0 {
		cfg.RequestTimeoutMs = 300_000
	}
	if cfg.LoginCacheMs == 0 {
		cfg.LoginCacheMs = 60_000
	}

	abs, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return Config{}, fmt.Errorf("resolve workspace: %w", err)
	}
	cfg.Workspace = abs

	return cfg, nil
}
