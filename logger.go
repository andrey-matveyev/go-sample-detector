package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"main/fdd"
	"os"
)

const (
	defaultLevel      slog.Level = slog.LevelInfo
	defaultAddSource  bool       = false
	defaultIsJSON     bool       = false
	defaultSetDefault bool       = false
)

type LoggerOptions struct {
	Level      slog.Level
	AddSource  bool
	IsJSON     bool
	SetDefault bool
	LogFile    *os.File
}

type LoggerOption func(*LoggerOptions)

func NewLogger(opts ...LoggerOption) *slog.Logger {
	// Create config by default
	config := &LoggerOptions{
		Level:      defaultLevel,
		AddSource:  defaultAddSource,
		IsJSON:     defaultIsJSON,
		SetDefault: defaultSetDefault,
		LogFile:    nil,
	}
	// Override by custom options
	for _, opt := range opts {
		opt(config)
	}

	// Applying default and custom options
	options := &slog.HandlerOptions{
		AddSource: config.AddSource,
		Level:     config.Level,
	}

	var writer io.Writer
	if config.LogFile == nil {
		writer = os.Stdout
	} else {
		writer = config.LogFile
	}

	var handler slog.Handler
	if config.IsJSON {
		handler = slog.NewJSONHandler(writer, options)
	} else {
		handler = slog.NewTextHandler(writer, options)
	}

	logger := slog.New(handler)

	if config.SetDefault {
		slog.SetDefault(logger)
	}

	return logger
}

func WithLevel(level string) LoggerOption {
	return func(opts *LoggerOptions) {
		var sl slog.Level
		err := sl.UnmarshalText([]byte(level))
		if err == nil {
			opts.Level = sl
		} else {
			opts.Level = slog.LevelInfo
		}
	}
}

func WithAddSource(addSource bool) LoggerOption {
	return func(opts *LoggerOptions) {
		opts.AddSource = addSource
	}
}

func WithIsJSON(isJSON bool) LoggerOption {
	return func(opts *LoggerOptions) {
		opts.IsJSON = isJSON
	}
}

func WithSetDefault(setDefault bool) LoggerOption {
	return func(opts *LoggerOptions) {
		opts.SetDefault = setDefault
	}
}

func WithLogFile(logFile *os.File) LoggerOption {
	return func(opts *LoggerOptions) {
		opts.LogFile = logFile
	}
}

// Create file for logging
func newLogFile(path string) *os.File {
	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666) // TODO: O_TRUNC
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return logFile
}

// Including logger to context.
func ContextWithLogger(ctx context.Context, key fdd.KeyCtxLogger, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, key, logger)
}
