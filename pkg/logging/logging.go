package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config configures the logger.
type Config struct {
	// minimum log level (debug, info, warn, error)
	Level string
	// log file path, empty for stdout only
	File string
	// max size in MB before rotation
	MaxSize int
	// max old log files to retain
	MaxBackups int
	// max days to retain old log files
	MaxAge int
	// gzip rotated files
	Compress bool
	// development mode (verbose, human-readable)
	Development bool
}

// DefaultConfig returns sensible defaults for edge deployment.
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
}

// New creates a new zap logger with the given configuration.
func New(cfg Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var encoderConfig zapcore.EncoderConfig
	if cfg.Development {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "ts"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	var cores []zapcore.Core

	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	consoleCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		level,
	)
	cores = append(cores, consoleCore)

	if cfg.File != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
		fileCore := zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(lj),
			level,
		)
		cores = append(cores, fileCore)
	}

	core := zapcore.NewTee(cores...)
	logger := zap.New(core)
	if cfg.Development {
		logger = logger.WithOptions(zap.AddCaller())
	}

	return logger, nil
}

// NewNop returns a no-op logger for testing.
func NewNop() *zap.Logger {
	return zap.NewNop()
}
