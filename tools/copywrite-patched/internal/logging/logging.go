// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type Level int

const (
	Trace Level = iota
	Debug
	Info
	Warn
	Error
	NoLevel
)

const (
	DefaultLevel = Info
	AutoColor    = false
)

type LoggerOptions struct {
	Name   string
	Level  Level
	Color  bool
	Output io.Writer
}

type StandardLoggerOptions struct {
	InferLevels bool
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Named(name string) Logger
	SetLevel(level Level)
	StandardLogger(opts *StandardLoggerOptions) *log.Logger
}

type simpleLogger struct {
	mu     sync.RWMutex
	name   string
	level  Level
	output io.Writer
}

var defaultLogger Logger = New(&LoggerOptions{})

func New(opts *LoggerOptions) Logger {
	if opts == nil {
		opts = &LoggerOptions{}
	}

	output := opts.Output
	if output == nil {
		output = os.Stdout
	}

	level := opts.Level
	if level < Trace || level > NoLevel {
		level = DefaultLevel
	}

	return &simpleLogger{
		name:   opts.Name,
		level:  level,
		output: output,
	}
}

func Default() Logger {
	return defaultLogger
}

func L() Logger {
	return defaultLogger
}

func SetDefault(logger Logger) {
	if logger != nil {
		defaultLogger = logger
	}
}

func LevelFromString(value string) Level {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TRACE":
		return Trace
	case "DEBUG":
		return Debug
	case "INFO":
		return Info
	case "WARN", "WARNING":
		return Warn
	case "ERROR":
		return Error
	default:
		return DefaultLevel
	}
}

func (l *simpleLogger) Named(name string) Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	fullName := name
	if l.name != "" && name != "" {
		fullName = l.name + "." + name
	} else if l.name != "" {
		fullName = l.name
	}

	return &simpleLogger{
		name:   fullName,
		level:  l.level,
		output: l.output,
	}
}

func (l *simpleLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *simpleLogger) Debug(msg string, args ...any) { l.log(Debug, msg, args...) }
func (l *simpleLogger) Info(msg string, args ...any)  { l.log(Info, msg, args...) }
func (l *simpleLogger) Error(msg string, args ...any) { l.log(Error, msg, args...) }

func (l *simpleLogger) StandardLogger(opts *StandardLoggerOptions) *log.Logger {
	return log.New(&standardLogWriter{logger: l, inferLevels: opts != nil && opts.InferLevels}, "", 0)
}

func (l *simpleLogger) log(level Level, msg string, args ...any) {
	l.mu.RLock()
	current := l.level
	name := l.name
	output := l.output
	l.mu.RUnlock()

	if current == NoLevel || level < current {
		return
	}

	if len(args) > 0 {
		msg = msg + " " + fmt.Sprint(args...)
	}

	prefix := "[" + levelName(level) + "]"
	if name != "" {
		prefix += " " + name + ":"
	}

	fmt.Fprintln(output, prefix, msg)
}

type standardLogWriter struct {
	logger      *simpleLogger
	inferLevels bool
	buffer      bytes.Buffer
}

func (w *standardLogWriter) Write(p []byte) (int, error) {
	_, err := w.buffer.Write(p)
	if err != nil {
		return 0, err
	}

	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			w.buffer.WriteString(line)
			break
		}
		w.writeLine(strings.TrimSuffix(line, "\n"))
	}

	return len(p), nil
}

func (w *standardLogWriter) writeLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	level := Info
	if w.inferLevels {
		if inferred, rest, ok := inferLevel(line); ok {
			level = inferred
			line = rest
		}
	}

	w.logger.log(level, line)
}

func inferLevel(line string) (Level, string, bool) {
	for _, prefix := range []struct {
		label string
		level Level
	}{
		{"[TRACE]", Trace},
		{"[DEBUG]", Debug},
		{"[INFO]", Info},
		{"[WARN]", Warn},
		{"[WARNING]", Warn},
		{"[ERROR]", Error},
	} {
		if strings.HasPrefix(line, prefix.label) {
			return prefix.level, strings.TrimSpace(strings.TrimPrefix(line, prefix.label)), true
		}
	}

	return Info, line, false
}

func levelName(level Level) string {
	switch level {
	case Trace:
		return "TRACE"
	case Debug:
		return "DEBUG"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "INFO"
	}
}
