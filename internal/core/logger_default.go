package core

import (
	"io"
	"log/slog"

	"github.com/pterm/pterm"
)

type DefaultLogger struct {
	level   LogLevel
	handler *slog.Logger
	output  io.Writer
}

func NewDefaultLogger(output io.Writer, level LogLevel) *DefaultLogger {
	var slogLevel slog.Level
	switch level {
	case LevelTrace, LevelDebug:
		slogLevel = slog.LevelDebug
	case LevelInfo:
		slogLevel = slog.LevelInfo
	case LevelWarn:
		slogLevel = slog.LevelWarn
	case LevelError:
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slogLevel,
	}))

	return &DefaultLogger{
		level:   level,
		handler: handler,
		output:  output,
	}
}

func (l *DefaultLogger) Trace(msg string, args ...any) {
	if l.level <= LevelTrace {
		pterm.Debug.WithWriter(l.output).Println("TRACE: " + msg)
		l.handler.Debug(msg, args...)
	}
}

func (l *DefaultLogger) Debug(msg string, args ...any) {
	if l.level <= LevelDebug {
		pterm.Debug.WithWriter(l.output).Println(msg)
		l.handler.Debug(msg, args...)
	}
}

func (l *DefaultLogger) Info(msg string, args ...any) {
	if l.level <= LevelInfo {
		pterm.Info.WithWriter(l.output).Println(msg)
		l.handler.Info(msg, args...)
	}
}

func (l *DefaultLogger) Warn(msg string, args ...any) {
	if l.level <= LevelWarn {
		pterm.Warning.WithWriter(l.output).Println(msg)
		l.handler.Warn(msg, args...)
	}
}

func (l *DefaultLogger) Error(msg string, args ...any) {
	if l.level <= LevelError {
		pterm.Error.WithWriter(l.output).Println(msg)
		l.handler.Error(msg, args...)
	}
}

func (l *DefaultLogger) With(args ...any) Logger {
	return &DefaultLogger{
		level:   l.level,
		handler: l.handler.With(args...),
		output:  l.output,
	}
}

func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
}
