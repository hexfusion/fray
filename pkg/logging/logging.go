package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger is the interface for structured logging.
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
	Sync() error
}

// zapLogger wraps *zap.Logger to implement Logger interface.
type zapLogger struct {
	*zap.Logger
}

func (z *zapLogger) With(fields ...zap.Field) Logger {
	return &zapLogger{z.Logger.With(fields...)}
}

// Wrap converts a *zap.Logger to the Logger interface.
func Wrap(l *zap.Logger) Logger {
	return &zapLogger{l}
}

// Nop returns a no-op logger.
func Nop() Logger {
	return &zapLogger{zap.NewNop()}
}

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

// New creates a new logger with the given configuration.
func New(cfg Config) (Logger, error) {
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

	return &zapLogger{logger}, nil
}

// NewNop returns a no-op logger for testing.
func NewNop() Logger {
	return &zapLogger{zap.NewNop()}
}

// NewConsole creates a console logger suitable for CLI use.
func NewConsole() Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.TimeKey = ""
	cfg.EncoderConfig.CallerKey = ""
	cfg.DisableStacktrace = true
	log, _ := cfg.Build()
	return &zapLogger{log}
}
