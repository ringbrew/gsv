package logger

import "log"

type MockLogger struct {
	level Level
}

func (m *MockLogger) SetLevel(level Level) {
	m.level = level
}

func (m *MockLogger) GetLevel() Level {
	return m.level
}

func (m *MockLogger) Debug(entry *LogEntry) {
	log.Printf("[DEBUG]%v-%v-%v-%v\n", entry.TraceId, entry.SpanId, entry.Extra, entry.Message)
}

func (m *MockLogger) Info(entry *LogEntry) {
	log.Printf("[INFO]%v-%v-%v-%v\n", entry.TraceId, entry.SpanId, entry.Extra, entry.Message)
}

func (m *MockLogger) Warn(entry *LogEntry) {
	log.Printf("[WARN]%v-%v-%v-%v\n", entry.TraceId, entry.SpanId, entry.Extra, entry.Message)
}

func (m *MockLogger) Error(entry *LogEntry) {
	log.Printf("[ERROR]%v-%v-%v-%v\n", entry.TraceId, entry.SpanId, entry.Extra, entry.Message)
}

func (m *MockLogger) Fatal(entry *LogEntry) {
	log.Printf("[FATAL]%v-%v-%v-%v\n", entry.TraceId, entry.SpanId, entry.Extra, entry.Message)
}

func (m *MockLogger) Close() error {
	return nil
}
