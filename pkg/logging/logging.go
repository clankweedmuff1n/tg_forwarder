package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"path/filepath"
)

// NewLogger sets up a Zap logger that outputs to both console (human-readable) and
// a JSON log file (log.jsonl) with rolling. Adjust as needed.
func NewLogger() *zap.SugaredLogger {
	// logs/log.jsonl
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		panic(err)
	}
	logFilePath := filepath.Join(logDir, "log.jsonl")

	fileRotation := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    5, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   false,
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	fileEncoder := zapcore.NewJSONEncoder(encoderCfg)
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)

	fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(fileRotation), zapcore.DebugLevel)
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapcore.InfoLevel)

	core := zapcore.NewTee(fileCore, consoleCore)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)) // skip 1 to get correct caller
	return logger.Sugar()
}
