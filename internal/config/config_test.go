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
