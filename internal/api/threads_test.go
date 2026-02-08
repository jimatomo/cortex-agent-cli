package api

import (
	"encoding/json"
	"testing"
)

func TestThreadIDToInt64(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		want    int64
		wantErr bool
	}{
		{"valid", "12345", 12345, false},
		{"zero", "0", 0, false},
		{"negative", "-1", -1, false},
		{"large", "9999999999999", 9999999999999, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
		{"float", "1.5", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ThreadIDToInt64(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ThreadIDToInt64(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}
}

func TestInt64ToThreadID(t *testing.T) {
	tests := []struct {
		name string
		id   int64
		want string
	}{
		{"positive", 12345, "12345"},
		{"zero", 0, "0"},
		{"negative", -1, "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Int64ToThreadID(tt.id)
			if got != tt.want {
				t.Errorf("Int64ToThreadID(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestThreadUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name:   "string thread_id",
			json:   `{"thread_id": "12345", "thread_name": "test"}`,
			wantID: "12345",
			wantName: "test",
		},
		{
			name:   "integer thread_id",
			json:   `{"thread_id": 67890, "thread_name": "test"}`,
			wantID: "67890",
			wantName: "test",
		},
		{
			name:   "null thread_id",
			json:   `{"thread_id": null, "thread_name": "test"}`,
			wantID: "",
			wantName: "test",
		},
		{
			name:    "invalid thread_id type",
			json:    `{"thread_id": [1,2,3]}`,
			wantErr: true,
		},
		{
			name:   "no thread_id field",
			json:   `{"thread_name": "test"}`,
			wantID: "",
			wantName: "test",
		},
		{
			name:   "with timestamps",
			json:   `{"thread_id": "999", "created_on": 1700000000000, "updated_on": 1700001000000}`,
			wantID: "999",
		},
		{
			name:    "invalid json",
			json:    `not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var thread Thread
			err := json.Unmarshal([]byte(tt.json), &thread)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if thread.ThreadID != tt.wantID {
				t.Errorf("ThreadID = %q, want %q", thread.ThreadID, tt.wantID)
			}
			if tt.wantName != "" && thread.ThreadName != tt.wantName {
				t.Errorf("ThreadName = %q, want %q", thread.ThreadName, tt.wantName)
			}
		})
	}
}
