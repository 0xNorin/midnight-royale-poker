// Package logging provides a minimal, size-limited rotating file logger for
// client diagnostics. It is deliberately not an observability framework: one
// file per edition, rotated when it grows past a bound, no network, no
// external service.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultMaxBytes = 512 << 10 // 512 KiB per file
	DefaultMaxFiles = 3         // keep the current file plus up to 3 rotated ones
)

// Logger is a concurrency-safe rotating file logger. Callers must never pass
// secrets (clipboard, codes, credentials, keys, artwork bytes) to it.
type Logger struct {
	mu       sync.Mutex
	dir      string
	name     string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
	debug    bool
}

// Open creates the directory if needed and opens (or creates) the log file.
func Open(dir, name string) (*Logger, error) {
	return OpenWithLimits(dir, name, DefaultMaxBytes, DefaultMaxFiles)
}

func OpenWithLimits(dir, name string, maxBytes int64, maxFiles int) (*Logger, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("logging: create directory: %w", err)
	}
	l := &Logger{dir: dir, name: name, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := l.reopen(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) reopen() error {
	if l.file != nil {
		_ = l.file.Close()
	}
	f, err := os.OpenFile(filepath.Join(l.dir, l.name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("logging: open file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("logging: stat file: %w", err)
	}
	l.file = f
	l.size = info.Size()
	return nil
}

func (l *Logger) rotateLocked() {
	_ = l.file.Close()
	for i := l.maxFiles - 1; i >= 1; i-- {
		old := filepath.Join(l.dir, fmt.Sprintf("%s.%d", l.name, i))
		newer := filepath.Join(l.dir, fmt.Sprintf("%s.%d", l.name, i+1))
		_ = os.Remove(newer)
		_ = os.Rename(old, newer)
	}
	_ = os.Rename(filepath.Join(l.dir, l.name), filepath.Join(l.dir, l.name+".1"))
	_ = l.reopen()
}

// Printf writes a timestamped line, rotating first when the current file would
// exceed the size bound.
func (l *Logger) Printf(format string, args ...any) {
	line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(line)) > l.maxBytes {
		l.rotateLocked()
	}
	if l.file != nil {
		if n, err := l.file.WriteString(line); err == nil {
			l.size += int64(n)
		}
	}
}

// Errorf logs an actionable error line.
func (l *Logger) Errorf(format string, args ...any) {
	l.Printf("error: "+format, args...)
}

// SetDebug toggles debug-level logging.
func (l *Logger) SetDebug(on bool) {
	l.mu.Lock()
	l.debug = on
	l.mu.Unlock()
}

// DebugEnabled reports whether debug-level logging is on.
func (l *Logger) DebugEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.debug
}

// Debugf logs only when debug mode is enabled.
func (l *Logger) Debugf(format string, args ...any) {
	l.mu.Lock()
	on := l.debug
	l.mu.Unlock()
	if !on {
		return
	}
	l.Printf("debug: "+format, args...)
}

// Path returns the current log file path.
func (l *Logger) Path() string { return filepath.Join(l.dir, l.name) }

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}
