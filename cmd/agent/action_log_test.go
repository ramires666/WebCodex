package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFormatActionRequest(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "initialize",
			input:    `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			contains: "Инициализация",
		},
		{
			name:     "shell_command",
			input:    `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"shell_command","arguments":{"command":"git status","workdir":"W:\\repo"}}}`,
			contains: "[shell] git status",
		},
		{
			name:     "apply_patch",
			input:    `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"apply_patch","arguments":{"input":"*** docs/plan.md\n--- docs/plan.md\n@@ -0,0 +1,2 @@\n+test"}}}`,
			contains: "[patch] docs/plan.md",
		},
		{
			name:     "read_file",
			input:    `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"C:/test/file.txt"}}}`,
			contains: "[read] C:/test/file.txt",
		},
		{
			name:     "write_file",
			input:    `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"C:/test/file.txt","content":"hello"}}}`,
			contains: "[write] C:/test/file.txt",
		},
		{
			name:     "update_plan",
			input:    `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"update_plan","arguments":{"plan":[{"step":"Download Databento","status":"in_progress"}]}}}`,
			contains: "[plan] Текущий шаг: Download Databento",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatActionRequest([]byte(tt.input))
			if !strings.Contains(got, tt.contains) {
				t.Errorf("formatActionRequest() = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

func TestFormatActionResponse(t *testing.T) {
	// 1. Success response
	respSuccess := []byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Exit code: 0\nOutput: ok"}],"isError":false}}`)
	gotSuccess := formatActionResponse(respSuccess, nil, 150*time.Millisecond)
	if !strings.Contains(gotSuccess, "Успешно") || !strings.Contains(gotSuccess, "Exit code: 0") {
		t.Errorf("got: %q", gotSuccess)
	}

	// 2. IsError response
	respError := []byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Exit code: 1\nFile not found"}],"isError":true}}`)
	gotError := formatActionResponse(respError, nil, 200*time.Millisecond)
	if !strings.Contains(gotError, "предупреждением/ошибкой") {
		t.Errorf("got: %q", gotError)
	}

	// 3. Network / Go error
	gotNetErr := formatActionResponse(nil, errors.New("timeout"), 2*time.Second)
	if !strings.Contains(gotNetErr, "Ошибка выполнения") {
		t.Errorf("got: %q", gotNetErr)
	}
}
