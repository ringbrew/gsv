package logger

import "context"

var l Logger

func SetLogger(ll Logger) {
	l = ll
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
	Message string
	Extra   map[string]string
}

func NewEntry(ctx ...context.Context) *LogEntry {
	return &LogEntry{
		Extra: make(map[string]string),
	}
}

func (entry *LogEntry) WithMessage(msg string) *LogEntry {
	entry.Message = msg
	return entry
}

func (entry *LogEntry) WithExtra(key string, value string) *LogEntry {
	entry.Extra[key] = value
	return entry
}

type Logger interface {
	SetLevel(level Level)
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
