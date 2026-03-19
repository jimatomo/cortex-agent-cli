package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCoragentConfig_ProjectRoot(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[eval]
output_dir = "./my-eval-output"
`
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644)

	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "./my-eval-output" {
		t.Errorf("expected ./my-eval-output, got %q", cfg.Eval.OutputDir)
	}
}

func TestLoadCoragentConfig_GlobalFallback(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })

	// Use an empty working directory (no .coragent.toml)
	workDir := filepath.Join(dir, "work")
	os.Mkdir(workDir, 0o755)
	os.Chdir(workDir)

	// Create ~/.coragent/config.toml
	fakeHome := filepath.Join(dir, "home")
	coragentDir := filepath.Join(fakeHome, ".coragent")
	os.MkdirAll(coragentDir, 0o755)
	content := `[eval]
output_dir = "/tmp/global-eval"
`
	os.WriteFile(filepath.Join(coragentDir, "config.toml"), []byte(content), 0o644)

	t.Setenv("HOME", fakeHome)

	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "/tmp/global-eval" {
		t.Errorf("expected /tmp/global-eval, got %q", cfg.Eval.OutputDir)
	}
}

func TestLoadCoragentConfig_ProjectRootPriority(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	// Project root config
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(`[eval]
output_dir = "project-dir"
`), 0o644)

	// Global config (should be ignored)
	fakeHome := filepath.Join(dir, "home")
	coragentDir := filepath.Join(fakeHome, ".coragent")
	os.MkdirAll(coragentDir, 0o755)
	os.WriteFile(filepath.Join(coragentDir, "config.toml"), []byte(`[eval]
output_dir = "global-dir"
`), 0o644)
	t.Setenv("HOME", fakeHome)

	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "project-dir" {
		t.Errorf("expected project-dir, got %q", cfg.Eval.OutputDir)
	}

	// Verify global config was not used
	globalCfg := LoadCoragentConfig()
	if globalCfg.Eval.OutputDir != "project-dir" {
		t.Errorf("expected project-dir (not global-dir), got %q", globalCfg.Eval.OutputDir)
	}
}

func TestLoadCoragentConfig_TimestampSuffix(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[eval]
output_dir = "./results"
timestamp_suffix = true
`
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644)

	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "./results" {
		t.Errorf("expected ./results, got %q", cfg.Eval.OutputDir)
	}
	if !cfg.Eval.TimestampSuffix {
		t.Error("expected TimestampSuffix to be true")
	}
}

func TestLoadCoragentConfig_TimestampSuffixDefault(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[eval]
output_dir = "./results"
`
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644)

	cfg := LoadCoragentConfig()
	if cfg.Eval.TimestampSuffix {
		t.Error("expected TimestampSuffix to be false by default")
	}
}

func TestLoadCoragentConfig_MalformedTOML(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	// Write a malformed TOML file
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte("[[invalid toml"), 0o644)

	// Should not panic; returns zero-value config
	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "" {
		t.Errorf("expected empty config for malformed TOML, got %q", cfg.Eval.OutputDir)
	}
}

func TestLoadCoragentConfig_NoFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	// Point HOME to empty dir so no fallback exists
	t.Setenv("HOME", dir)

	cfg := LoadCoragentConfig()
	if cfg.Eval.OutputDir != "" {
		t.Errorf("expected empty string, got %q", cfg.Eval.OutputDir)
	}
}

func TestLoadCoragentConfig_FeedbackRemote(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[feedback.remote]
enabled = true
database = "MY_DB"
schema = "MY_SCHEMA"
table = "AGENT_FEEDBACK"
`
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644)

	cfg := LoadCoragentConfig()
	if !cfg.Feedback.Remote.Enabled {
		t.Error("expected Feedback.Remote.Enabled to be true")
	}
	if cfg.Feedback.Remote.Database != "MY_DB" {
		t.Errorf("expected database MY_DB, got %q", cfg.Feedback.Remote.Database)
	}
	if cfg.Feedback.Remote.Schema != "MY_SCHEMA" {
		t.Errorf("expected schema MY_SCHEMA, got %q", cfg.Feedback.Remote.Schema)
	}
	if cfg.Feedback.Remote.Table != "AGENT_FEEDBACK" {
		t.Errorf("expected table AGENT_FEEDBACK, got %q", cfg.Feedback.Remote.Table)
	}
}

func TestLoadCoragentConfig_FeedbackRemoteDefault(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[eval]
output_dir = "./out"
`
	os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644)

	cfg := LoadCoragentConfig()
	if cfg.Feedback.Remote.Enabled {
		t.Error("expected Feedback.Remote.Enabled to be false by default")
	}
	if cfg.Feedback.Remote.Database != "" || cfg.Feedback.Remote.Schema != "" || cfg.Feedback.Remote.Table != "" {
		t.Errorf("expected empty feedback.remote db/schema/table by default, got %q/%q/%q",
			cfg.Feedback.Remote.Database, cfg.Feedback.Remote.Schema, cfg.Feedback.Remote.Table)
	}
}

func TestLoadCoragentConfig_QueryTagBase(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	content := `[query_tag]
base = "team-cli"
`
	if err := os.WriteFile(filepath.Join(dir, ".coragent.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadCoragentConfig()
	if cfg.QueryTag.Base != "team-cli" {
		t.Errorf("expected query tag base team-cli, got %q", cfg.QueryTag.Base)
	}
}

func TestLoadCoragentConfig_QueryTagBaseGlobalFallback(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })

	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	os.Chdir(workDir)

	fakeHome := filepath.Join(dir, "home")
	coragentDir := filepath.Join(fakeHome, ".coragent")
	if err := os.MkdirAll(coragentDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `[query_tag]
base = "global-base"
`
	if err := os.WriteFile(filepath.Join(coragentDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	t.Setenv("HOME", fakeHome)

	cfg := LoadCoragentConfig()
	if cfg.QueryTag.Base != "global-base" {
		t.Errorf("expected global query tag base, got %q", cfg.QueryTag.Base)
	}
}
