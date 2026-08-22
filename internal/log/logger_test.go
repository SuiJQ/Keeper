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

func TestLoggerWithTrace(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	traceLogger := logger.WithTrace("trace-123", "span-456")
	traceLogger.Info("trace test", Field{Key: "key", Value: "value"})

	output := buf.String()
	if !strings.Contains(output, "trace-123") {
		t.Errorf("Logger.WithTrace() output missing trace_id: %s", output)
	}
	if !strings.Contains(output, "span-456") {
		t.Errorf("Logger.WithTrace() output missing span_id: %s", output)
	}
}

func TestLoggerWithTraceOverwrite(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	// 先设置 trace，再覆盖
	traceLogger := logger.WithTrace("trace-1", "span-1")
	traceLogger2 := traceLogger.WithTrace("trace-2", "span-2")
	traceLogger2.Info("trace test")

	output := buf.String()
	if strings.Contains(output, "trace-1") {
		t.Errorf("Logger.WithTrace() should overwrite trace_id: %s", output)
	}
	if !strings.Contains(output, "trace-2") {
		t.Errorf("Logger.WithTrace() output missing new trace_id: %s", output)
	}
}

func TestLoggerWithTraceEmpty(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	// 空 trace ID 不应添加字段
	traceLogger := logger.WithTrace("", "")
	traceLogger.Info("no trace test")

	output := buf.String()
	if strings.Contains(output, "trace_id") {
		t.Errorf("Logger.WithTrace() should not add empty trace_id: %s", output)
	}
	if strings.Contains(output, "span_id") {
		t.Errorf("Logger.WithTrace() should not add empty span_id: %s", output)
	}
}

func TestGenerateTraceID(t *testing.T) {
	id1 := GenerateTraceID()
	id2 := GenerateTraceID()

	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("GenerateTraceID() length = %d, want 32", len(id1))
	}
	if len(id2) != 32 {
		t.Errorf("GenerateTraceID() length = %d, want 32", len(id2))
	}
	if id1 == id2 {
		t.Errorf("GenerateTraceID() should generate unique IDs")
	}
}

func TestGenerateSpanID(t *testing.T) {
	id1 := GenerateSpanID()
	id2 := GenerateSpanID()

	if len(id1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("GenerateSpanID() length = %d, want 16", len(id1))
	}
	if len(id2) != 16 {
		t.Errorf("GenerateSpanID() length = %d, want 16", len(id2))
	}
	if id1 == id2 {
		t.Errorf("GenerateSpanID() should generate unique IDs")
	}
}

func TestNextTraceID(t *testing.T) {
	id1 := NextTraceID()
	id2 := NextTraceID()

	if len(id1) != 32 {
		t.Errorf("NextTraceID() length = %d, want 32", len(id1))
	}
	if id1 == id2 {
		t.Errorf("NextTraceID() should generate unique IDs")
	}
}

func TestNextSpanID(t *testing.T) {
	id1 := NextSpanID()
	id2 := NextSpanID()

	if len(id1) != 16 {
		t.Errorf("NextSpanID() length = %d, want 16", len(id1))
	}
	if id1 == id2 {
		t.Errorf("NextSpanID() should generate unique IDs")
	}
}
