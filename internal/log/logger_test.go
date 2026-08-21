package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	if logger == nil {
		t.Error("New() returned nil")
	}
}

func TestNewLoggerNilOutput(t *testing.T) {
	logger := New(nil)
	if logger == nil {
		t.Error("New(nil) returned nil")
	}
}

func TestLoggerInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Info("test message", Field{Key: "key1", Value: "value1"})

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Logger.Info() output missing message: %s", output)
	}
	if !strings.Contains(output, "key1") {
		t.Errorf("Logger.Info() output missing field key: %s", output)
	}
	if !strings.Contains(output, "value1") {
		t.Errorf("Logger.Info() output missing field value: %s", output)
	}
	if !strings.Contains(output, `"level":"info"`) {
		t.Errorf("Logger.Info() output missing level: %s", output)
	}
}

func TestLoggerDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Debug("debug message", Field{Key: "debug_key", Value: "debug_value"})

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("Logger.Debug() output missing message: %s", output)
	}
	if !strings.Contains(output, `"level":"debug"`) {
		t.Errorf("Logger.Debug() output missing level: %s", output)
	}
}

func TestLoggerWarn(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Warn("warn message", Field{Key: "warn_key", Value: "warn_value"})

	output := buf.String()
	if !strings.Contains(output, "warn message") {
		t.Errorf("Logger.Warn() output missing message: %s", output)
	}
	if !strings.Contains(output, `"level":"warn"`) {
		t.Errorf("Logger.Warn() output missing level: %s", output)
	}
}

func TestLoggerError(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Error("error message", Field{Key: "error_key", Value: "error_value"})

	output := buf.String()
	if !strings.Contains(output, "error message") {
		t.Errorf("Logger.Error() output missing message: %s", output)
	}
	if !strings.Contains(output, `"level":"error"`) {
		t.Errorf("Logger.Error() output missing level: %s", output)
	}
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	childLogger := logger.WithFields(
		Field{Key: "global_key", Value: "global_value"},
	)
	childLogger.Info("child message", Field{Key: "local_key", Value: "local_value"})

	output := buf.String()
	if !strings.Contains(output, "global_value") {
		t.Errorf("Logger.WithFields() output missing parent field: %s", output)
	}
	if !strings.Contains(output, "local_value") {
		t.Errorf("Logger.WithFields() output missing child field: %s", output)
	}
}

func TestLoggerWithFieldsOverwrite(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	childLogger := logger.WithFields(
		Field{Key: "key", Value: "parent"},
	)
	childLogger.Info("message", Field{Key: "key", Value: "child"})

	output := buf.String()
	if !strings.Contains(output, `"key":"child"`) {
		t.Errorf("Logger.WithFields() did not overwrite field: %s", output)
	}
}

func TestLoggerSetOutput(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer
	logger := New(&buf1)

	logger.SetOutput(&buf2)
	logger.Info("test output")

	if buf1.Len() > 0 {
		t.Errorf("Logger.SetOutput() did not switch output, buf1 still has: %s", buf1.String())
	}
	if buf2.Len() == 0 {
		t.Error("Logger.SetOutput() did not write to new output")
	}
}

func TestLoggerSync(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	err := logger.Sync()
	if err != nil {
		t.Errorf("Logger.Sync() returned error: %v", err)
	}
}

func TestGlobalLogger(t *testing.T) {
	var buf bytes.Buffer
	oldGlobal := Global()
	defer SetGlobal(oldGlobal)

	newLogger := New(&buf)
	SetGlobal(newLogger)

	Global().Info("global test")

	output := buf.String()
	if !strings.Contains(output, "global test") {
		t.Errorf("Global() logger did not receive message: %s", output)
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	logger.Info("json test", Field{Key: "number", Value: 42})

	output := buf.String()
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("Logger output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if entry["level"] != "info" {
		t.Errorf("JSON level = %v, want info", entry["level"])
	}
	if entry["message"] != "json test" {
		t.Errorf("JSON message = %v, want json test", entry["message"])
	}
	fields := entry["fields"].(map[string]interface{})
	if fields["number"] != "42" {
		t.Errorf("JSON fields[number] = %v, want \"42\"", fields["number"])
	}
}
