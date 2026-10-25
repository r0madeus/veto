package core

import (
	"io"
	"log/slog"

	"github.com/pterm/pterm"
)

type DefaultLogger struct {
	level   LogLevel
	slogLvl *slog.LevelVar // shared with handler so SetLevel takes effect
	handler *slog.Logger
	output  io.Writer
}

func toSlogLevel(level LogLevel) slog.Level {
	switch level {
	case LevelTrace, LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func NewDefaultLogger(output io.Writer, level LogLevel) *DefaultLogger {
	lvl := &slog.LevelVar{}
	lvl.Set(toSlogLevel(level))

	handler := slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: lvl,
	}))

	return &DefaultLogger{
		level:   level,
		slogLvl: lvl,
		handler: handler,
		output:  output,
	}
}

func (l *DefaultLogger) Trace(msg string, args ...any) {
	if l.level <= LevelTrace {
		pterm.Debug.WithDebugger(false).WithWriter(l.output).Println("TRACE: " + msg)
		l.handler.Debug(msg, args...)
	}
}

func (l *DefaultLogger) Debug(msg string, args ...any) {
	if l.level <= LevelDebug {
		pterm.Debug.WithDebugger(false).WithWriter(l.output).Println(msg)
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
		slogLvl: l.slogLvl,
		handler: l.handler.With(args...),
		output:  l.output,
	}
}

func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
	l.slogLvl.Set(toSlogLevel(level))
}
