// Package log 提供结构化日志接口
package log

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level 日志级别
type Level string

const (
	// LevelDebug debug level
	LevelDebug Level = "debug"
	// LevelInfo info level
	LevelInfo Level = "info"
	// LevelWarn warn level
	LevelWarn Level = "warn"
	// LevelError error level
	LevelError Level = "error"
	// LevelFatal fatal level
	LevelFatal Level = "fatal"
)

// Field 日志字段
type Field struct {
	Key   string
	Value interface{}
}

// Logger 结构化日志接口
type Logger interface {
	// 基础日志方法
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)

	// 子日志器（带上下文）
	WithFields(fields ...Field) Logger

	// Trace 子日志器（带 trace 上下文）
	WithTrace(traceID, spanID string) Logger

	// 同步刷盘
	Sync() error

	// 设置输出目标
	SetOutput(w io.Writer)
}

// 预定义字段键
const (
	FieldAgentName = "agent_name"
	FieldState     = "state"
	FieldPID       = "pid"
	FieldPGID      = "pgid"
	FieldError     = "error"
	FieldDuration  = "duration_ms"
	FieldCommand   = "command"
	FieldStage     = "stage"
	FieldTimestamp = "timestamp"
	FieldTraceID   = "trace_id"
	FieldSpanID    = "span_id"
)

// entry 日志条目
type entry struct {
	Level   Level             `json:"level"`
	Message string            `json:"message"`
	Time    time.Time         `json:"timestamp"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// logger 日志实现
type logger struct {
	output  io.Writer
	fields  map[string]string
	traceID string
	spanID  string
}

// New 创建新的 Logger 实例
func New(output io.Writer) Logger {
	if output == nil {
		output = os.Stdout
	}

	return &logger{
		output: output,
		fields: make(map[string]string),
	}
}

// WithFields 创建带上下文的子日志器
func (l *logger) WithFields(fields ...Field) Logger {
	newFields := make(map[string]string, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}

	for _, f := range fields {
		newFields[f.Key] = fmt.Sprintf("%v", f.Value)
	}

	return &logger{
		output:  l.output,
		fields:  newFields,
		traceID: l.traceID,
		spanID:  l.spanID,
	}
}

// WithTrace 创建带 trace 上下文的子日志器
func (l *logger) WithTrace(traceID, spanID string) Logger {
	newFields := make(map[string]string, len(l.fields))
	for k, v := range l.fields {
		newFields[k] = v
	}

	if traceID != "" {
		newFields[FieldTraceID] = traceID
	}
	if spanID != "" {
		newFields[FieldSpanID] = spanID
	}

	return &logger{
		output:  l.output,
		fields:  newFields,
		traceID: traceID,
		spanID:  spanID,
	}
}

// log 内部日志方法
func (l *logger) log(level Level, msg string, fields ...Field) {
	entry := entry{
		Level:   level,
		Message: msg,
		Time:    time.Now().UTC(),
		Fields:  make(map[string]string),
	}

	// 复制基础字段
	for k, v := range l.fields {
		entry.Fields[k] = v
	}

	// 添加临时字段
	for _, f := range fields {
		entry.Fields[f.Key] = fmt.Sprintf("%v", f.Value)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// 日志系统本身失败，输出到 stderr
		fmt.Fprintf(os.Stderr, "LOG_SERIALIZE_ERROR: %v\n", err)
		return
	}

	_, _ = fmt.Fprintln(l.output, string(data))
}

// Debug 调试日志
func (l *logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Info 信息日志
func (l *logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Warn 警告日志
func (l *logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Error 错误日志
func (l *logger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// Fatal 致命错误日志（然后退出）
func (l *logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields...)
	os.Exit(1)
}

// Sync 同步输出（对于某些 Writer 需要）
func (l *logger) Sync() error {
	if syncer, ok := l.output.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}
	return nil
}

// SetOutput 设置输出目标
func (l *logger) SetOutput(w io.Writer) {
	l.output = w
}

// 全局日志器实例
var defaultLogger Logger = New(os.Stdout)

// SetGlobal 设置全局日志器
func SetGlobal(logger Logger) {
	defaultLogger = logger
}

// Global 获取全局日志器
func Global() Logger {
	return defaultLogger
}

// GenerateTraceID 生成新的 trace ID（16 字节随机 hex）
func GenerateTraceID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// GenerateSpanID 生成新的 span ID（8 字节随机 hex）
func GenerateSpanID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// TraceID 全局 trace ID 生成器（带缓存）
var traceIDGenerator = &traceIDGen{
	mu:    &sync.Mutex{},
	state: make([]byte, 16),
}

type traceIDGen struct {
	mu    *sync.Mutex
	state []byte
}

func (g *traceIDGen) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, _ = rand.Read(g.state)
	return hex.EncodeToString(g.state)
}

// NextTraceID 生成全局唯一 trace ID
func NextTraceID() string {
	return traceIDGenerator.Next()
}

// NextSpanID 生成全局唯一 span ID
func NextSpanID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
