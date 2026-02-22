package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CoragentConfig represents the top-level structure of .coragent.toml.
type CoragentConfig struct {
	Eval EvalSettings `toml:"eval"`
}

// EvalSettings holds eval-specific configuration.
type EvalSettings struct {
	OutputDir              string   `toml:"output_dir"`
	TimestampSuffix        bool     `toml:"timestamp_suffix"`
	JudgeModel             string   `toml:"judge_model"`
	ResponseScoreThreshold int      `toml:"response_score_threshold"`
	IgnoreTools            []string `toml:"ignore_tools"`
}

// LoadCoragentConfig loads configuration from .coragent.toml.
// Search order:
//  1. Current directory: .coragent.toml
//  2. ~/.coragent/config.toml
//
// If no file is found, a zero-value config is returned.
func LoadCoragentConfig() CoragentConfig {
	var cfg CoragentConfig

	// 1. Current directory
	if _, err := os.Stat(".coragent.toml"); err == nil {
		if _, err := toml.DecodeFile(".coragent.toml", &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: .coragent.toml parse error: %v\n", err)
		}
		return cfg
	}

	// 2. ~/.coragent/config.toml
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	globalPath := filepath.Join(home, ".coragent", "config.toml")
	if _, err := os.Stat(globalPath); err == nil {
		if _, err := toml.DecodeFile(globalPath, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s parse error: %v\n", globalPath, err)
		}
		return cfg
	}

	return cfg
}
