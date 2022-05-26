package logger

import "log"

type MockLogger struct {
	level Level
}

func (m *MockLogger) SetLevel(level Level) {
	m.level = level
}

func (m MockLogger) Debug(entry *LogEntry) {
	log.Printf("[DEBUG]%v\n", entry.Message)
}

func (m MockLogger) Info(entry *LogEntry) {
	log.Printf("[INFO]%v\n", entry.Message)
}

func (m MockLogger) Warn(entry *LogEntry) {
	log.Printf("[WARN]%v\n", entry.Message)
}

func (m MockLogger) Error(entry *LogEntry) {
	log.Printf("[ERROR]%v\n", entry.Message)
}

func (m MockLogger) Fatal(entry *LogEntry) {
	log.Printf("[FATAL]%v\n", entry.Message)
}

func (m MockLogger) Close() error {
	return nil
}
