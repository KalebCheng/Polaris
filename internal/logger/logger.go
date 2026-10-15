// Package logger 提供全局结构化日志（zap）。
// 属于 §3.1-1 允许的"跨切面基础设施"：进程唯一、无业务状态，在启动引导阶段初始化一次。
package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
	mu           sync.RWMutex
)

// Config 日志配置
type Config struct {
	Level  string // debug/info/warn/error
	Format string // json/console
	Output string // stdout/file
}

// Init 初始化全局日志（幂等）
func Init(cfg Config) error {
	var err error
	once.Do(func() {
		globalLogger, err = newLogger(cfg)
	})
	return err
}

func newLogger(cfg Config) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	writeSyncer := zapcore.AddSync(os.Stdout)
	core := zapcore.NewCore(encoder, writeSyncer, level)
	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// L 获取全局日志实例
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if globalLogger == nil {
		return zap.NewNop()
	}
	return globalLogger
}

// SetLogger 设置全局日志实例
func SetLogger(l *zap.Logger) {
	mu.Lock()
	defer mu.Unlock()
	globalLogger = l
}

// Debug 调试日志
func Debug(msg string, keysAndValues ...interface{}) { L().Sugar().Debugw(msg, keysAndValues...) }

// Info 信息日志
func Info(msg string, keysAndValues ...interface{}) { L().Sugar().Infow(msg, keysAndValues...) }

// Warn 警告日志
func Warn(msg string, keysAndValues ...interface{}) { L().Sugar().Warnw(msg, keysAndValues...) }

// Error 错误日志
func Error(msg string, keysAndValues ...interface{}) { L().Sugar().Errorw(msg, keysAndValues...) }

// Fatal 致命错误日志
func Fatal(msg string, keysAndValues ...interface{}) { L().Sugar().Fatalw(msg, keysAndValues...) }

// Sync 同步日志缓冲区
func Sync() error { return L().Sync() }
