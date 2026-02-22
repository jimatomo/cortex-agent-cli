package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runValidateCmd(opts *RootOptions, args []string) (string, error) {
	var buf bytes.Buffer
	cmd := newValidateCmd(opts)
	cmd.SetOut(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestValidateCmdValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("name: test-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := runValidateCmd(&RootOptions{}, []string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok: "+path) {
		t.Errorf("output %q does not contain %q", out, "ok: "+path)
	}
}

func TestValidateCmdMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("name: agent-a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("name: agent-b\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := runValidateCmd(&RootOptions{}, []string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 ok lines, got %d: %q", len(lines), out)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "ok: ") {
			t.Errorf("expected line to start with 'ok: ', got: %q", line)
		}
	}
}

func TestValidateCmdInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("key: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := runValidateCmd(&RootOptions{}, []string{path})
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestValidateCmdNonExistentPath(t *testing.T) {
	_, err := runValidateCmd(&RootOptions{}, []string{"/nonexistent/path/agent.yaml"})
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

func TestValidateCmdMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("comment: no name field\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := runValidateCmd(&RootOptions{}, []string{path})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error about 'name', got: %v", err)
	}
}

func TestValidateCmdUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("name: test\nunknown_field: oops\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := runValidateCmd(&RootOptions{}, []string{path})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestValidateCmdDefaultPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("name: test-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Chdir(dir)

	out, err := runValidateCmd(&RootOptions{}, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok:") {
		t.Errorf("output %q missing 'ok:' prefix", out)
	}
}

func TestValidateCmdEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	_, err := runValidateCmd(&RootOptions{}, []string{dir})
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}

func TestValidateCmdRecursive(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.yaml"), []byte("name: root-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "child.yaml"), []byte("name: child-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := runValidateCmd(&RootOptions{}, []string{"-R", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 ok lines (recursive), got %d: %q", len(lines), out)
	}
}

func TestValidateCmdNonRecursiveIgnoresSubdirs(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.yaml"), []byte("name: root-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "child.yaml"), []byte("name: child-agent\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := runValidateCmd(&RootOptions{}, []string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 ok line (non-recursive), got %d: %q", len(lines), out)
	}
}

func TestValidateCmdWithEnvVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(`
vars:
  dev:
    SNOWFLAKE_DATABASE: DEV_DB
  default:
    SNOWFLAKE_DATABASE: PROD_DB
name: test-agent
deploy:
  database: ${ vars.SNOWFLAKE_DATABASE }
`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := runValidateCmd(&RootOptions{Env: "dev"}, []string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok: "+path) {
		t.Errorf("output %q does not contain %q", out, "ok: "+path)
	}
}

func TestValidateCmdInvalidGrantPrivilege(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ANALYST_ROLE
        privileges:
          - INVALID_PRIV
`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := runValidateCmd(&RootOptions{}, []string{path})
	if err == nil {
		t.Fatal("expected error for invalid grant privilege, got nil")
	}
	if !strings.Contains(err.Error(), "invalid privilege") {
		t.Errorf("expected error about 'invalid privilege', got: %v", err)
	}
}
