package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the GitHub target configuration.
// Authentication is handled via the GITHUB_TOKEN environment variable,
// not stored in the config.
type Config struct {
	Type         string `json:"Type"`
	ApiUrl       string `json:"ApiUrl,omitempty"`
	Organization string `json:"Organization,omitempty"`
}

// Token returns the GitHub personal access token from the environment.
func (c *Config) Token() (string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}
	return token, nil
}

// BaseURL returns the API base URL, defaulting to https://api.github.com.
func (c *Config) BaseURL() string {
	if c.ApiUrl != "" {
		return c.ApiUrl
	}
	return "https://api.github.com"
}

// FromTargetConfig deserializes a target config JSON into a Config.
func FromTargetConfig(targetConfig json.RawMessage) *Config {
	if targetConfig == nil {
		return &Config{}
	}
	cfg := &Config{}
	_ = json.Unmarshal(targetConfig, cfg)
	return cfg
}
