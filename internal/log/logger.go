// Package log 提供结构化日志接口
package log

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Level 日志级别
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
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

	// 同步刷盘
	Sync() error

	// 设置输出目标
	SetOutput(w io.Writer)
}

// 预定义字段键
const (
	FieldAgentName   = "agent_name"
	FieldState       = "state"
	FieldPID         = "pid"
	FieldPGID        = "pgid"
	FieldError       = "error"
	FieldDuration    = "duration_ms"
	FieldCommand     = "command"
	FieldStage       = "stage"
	FieldTimestamp   = "timestamp"
)

// entry 日志条目
type entry struct {
	Level   Level            `json:"level"`
	Message string            `json:"message"`
	Time    time.Time         `json:"timestamp"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// logger 日志实现
type logger struct {
	output io.Writer
	fields map[string]string
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
		output: l.output,
		fields: newFields,
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

	fmt.Fprintln(l.output, string(data))
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
