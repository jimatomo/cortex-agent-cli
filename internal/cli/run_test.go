package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just now", 5 * time.Second, "just now"},
		{"30 seconds", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"59 minutes", 59 * time.Minute, "59 minutes ago"},
		{"1 hour", 1 * time.Hour, "1 hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"23 hours", 23 * time.Hour, "23 hours ago"},
		{"1 day", 25 * time.Hour, "1 day ago"},
		{"3 days", 73 * time.Hour, "3 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAge(time.Now().Add(-tt.ago))
			if got != tt.want {
				t.Errorf("formatAge(%v ago) = %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}

func TestTruncateDisplay(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},
		{"very short max", "abcdef", 4, "a..."},
		{"empty", "", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateDisplay(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateDisplay(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"short", "hello", "hello"},
		{"exactly 100", strings.Repeat("x", 100), strings.Repeat("x", 100)},
		{"101 chars", strings.Repeat("x", 101), strings.Repeat("x", 97) + "..."},
		{"newlines replaced", "hello\nworld\nfoo", "hello world foo"},
		{"trimmed", "  hello  ", "hello"},
		{"long with newlines", strings.Repeat("a", 50) + "\n" + strings.Repeat("b", 55),
			strings.Repeat("a", 50) + " " + strings.Repeat("b", 46) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateSummary(tt.s)
			if got != tt.want {
				t.Errorf("truncateSummary(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"seconds", 5 * time.Second, "5s"},
		{"59 seconds", 59 * time.Second, "59s"},
		{"1 minute", 60 * time.Second, "1m"},
		{"1 minute 30 seconds", 90 * time.Second, "1m30s"},
		{"2 minutes", 120 * time.Second, "2m"},
		{"5 minutes 5 seconds", 305 * time.Second, "5m5s"},
		{"sub-second truncated", 500 * time.Millisecond, "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatElapsed(tt.d)
			if got != tt.want {
				t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestTruncateResult(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{"short", json.RawMessage(`{"key": "value"}`), `{"key": "value"}`},
		{"exactly 200", json.RawMessage(strings.Repeat("x", 200)), strings.Repeat("x", 200)},
		{"201 chars", json.RawMessage(strings.Repeat("x", 201)), strings.Repeat("x", 200) + "..."},
		{"empty", json.RawMessage(""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateResult(tt.data)
			if got != tt.want {
				t.Errorf("truncateResult len=%d got len=%d", len(tt.want), len(got))
			}
		})
	}
}
