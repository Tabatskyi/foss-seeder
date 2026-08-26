package logger

import (
	"fmt"
	"sync"
	"time"
)

type Level string

const (
	LevelInfo    Level = "INFO"
	LevelSuccess Level = "SUCCESS"
	LevelWarn    Level = "WARN"
	LevelError   Level = "ERROR"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     Level  `json:"level"`
	Message   string `json:"message"`
}

type Logger struct {
	mu          sync.RWMutex
	history     []LogEntry
	maxHistory  int
	subscribers map[chan LogEntry]struct{}
}

var defaultLogger = New(200)

func New(maxHistory int) *Logger {
	return &Logger{
		history:     make([]LogEntry, 0, maxHistory),
		maxHistory:  maxHistory,
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

func GetDefault() *Logger {
	return defaultLogger
}

func (l *Logger) Log(lvl Level, format string, args ...any) {
	var msg string
	if len(args) == 0 {
		msg = format
	} else {
		msg = fmt.Sprintf(format, args...)
	}
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     lvl,
		Message:   msg,
	}

	fmt.Printf("[%s] [%s] %s\n", entry.Timestamp, entry.Level, entry.Message)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.history) >= l.maxHistory {
		l.history = l.history[1:]
	}
	l.history = append(l.history, entry)

	for ch := range l.subscribers {
		select {
		case ch <- entry:
		default:
			// Client buffer full, skip to prevent blocking
		}
	}
}

func (l *Logger) Info(format string, args ...any) {
	l.Log(LevelInfo, format, args...)
}

func (l *Logger) Success(format string, args ...any) {
	l.Log(LevelSuccess, format, args...)
}

func (l *Logger) Warn(format string, args ...any) {
	l.Log(LevelWarn, format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.Log(LevelError, format, args...)
}

func (l *Logger) History() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]LogEntry, len(l.history))
	copy(out, l.history)
	return out
}

func (l *Logger) Subscribe() chan LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	ch := make(chan LogEntry, 50)
	l.subscribers[ch] = struct{}{}
	return ch
}

func (l *Logger) Unsubscribe(ch chan LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.subscribers, ch)
	close(ch)
}

// Global helper wrappers
func Info(format string, args ...any)    { defaultLogger.Info(format, args...) }
func Success(format string, args ...any) { defaultLogger.Success(format, args...) }
func Warn(format string, args ...any)    { defaultLogger.Warn(format, args...) }
func Error(format string, args ...any)   { defaultLogger.Error(format, args...) }
