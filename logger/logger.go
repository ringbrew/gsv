package logger

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/service"
)

var l Logger

func SetLogger(ll Logger) {
	l = ll
}

func SetLevel(ll Level) {
	l.SetLevel(ll)
}

func GetLevel() Level {
	return l.GetLevel()
}

type Level uint8

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelOff
)

type LogEntry struct {
	TraceId  string
	SpanId   string
	ParentId string
	Message  string
	Extra    map[string]interface{}
}

func NewEntry(ctx ...context.Context) *LogEntry {
	result := &LogEntry{
		Extra: make(map[string]interface{}),
	}

	if len(ctx) > 0 {
		if rpcCtx, ok := service.FromContext(ctx[0]); ok {
			result.TraceId = rpcCtx.TraceId()
			result.SpanId = rpcCtx.SpanId()
			result.ParentId = rpcCtx.ParentId()
		}
	}
	return result
}

func (entry *LogEntry) WithMessage(msg string) *LogEntry {
	entry.Message = msg
	return entry
}

func (entry *LogEntry) WithMessageF(format string, a ...interface{}) *LogEntry {
	entry.Message = fmt.Sprintf(format, a...)
	return entry
}

func (entry *LogEntry) WithExtra(key string, value interface{}) *LogEntry {
	entry.Extra[key] = value
	return entry
}

type Logger interface {
	SetLevel(level Level)
	GetLevel() Level
	Debug(entry *LogEntry)
	Info(entry *LogEntry)
	Warn(entry *LogEntry)
	Error(entry *LogEntry)
	Fatal(entry *LogEntry)
	Close() error
}

func NewDefaultLogger() Logger {
	return &MockLogger{}
}

func init() {
	l = NewDefaultLogger()
}

func Debug(entry *LogEntry) {
	l.Debug(entry)
}

func Info(entry *LogEntry) {
	l.Info(entry)
}

func Warn(entry *LogEntry) {
	l.Warn(entry)
}

func Error(entry *LogEntry) {
	l.Error(entry)
}

func Fatal(entry *LogEntry) {
	l.Fatal(entry)
}
