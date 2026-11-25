package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_WritesConsoleAndRotatingJSONFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "server.log")
	var consoleBuf bytes.Buffer

	log, closeFn, err := New("development", logFile, &consoleBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = closeFn() }()

	log.Debug("debug message", "key", "val")
	log.Info("info message")

	if !strings.Contains(consoleBuf.String(), "debug message") {
		t.Fatalf("expected debug message in console output, got %q", consoleBuf.String())
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSON log lines, got %d", len(lines))
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("expected valid JSON log line: %v", err)
	}
	if parsed["service"] != "DashboardServer" {
		t.Fatalf("expected service=DashboardServer, got %v", parsed["service"])
	}
}

func TestNew_ProductionSkipsDebug(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "server.log")
	var consoleBuf bytes.Buffer

	log, closeFn, err := New("production", logFile, &consoleBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = closeFn() }()

	log.Debug("should not appear")
	log.Info("should appear")

	if strings.Contains(consoleBuf.String(), "should not appear") {
		t.Fatalf("did not expect debug log in production")
	}
	if !strings.Contains(consoleBuf.String(), "should appear") {
		t.Fatalf("expected info log in production")
	}
}
