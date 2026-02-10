package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggerCfg struct {
	LogFilePath    string
	Stdout         bool
	LoggingEnabled bool
}

func (cfg *LoggerCfg) WithLogFilePath(path string) *LoggerCfg {
	cfg.LogFilePath = path
	return cfg
}

func (cfg *LoggerCfg) WithStdout(stdout bool) *LoggerCfg {
	cfg.Stdout = stdout
	return cfg
}

func (cfg *LoggerCfg) WithLoggingEnabled(enabled bool) *LoggerCfg {
	cfg.LoggingEnabled = enabled
	return cfg
}

func NewLoggerCfg() *LoggerCfg {
	return &LoggerCfg{
		LogFilePath:    "logs/bore.log",
		Stdout:         true,
		LoggingEnabled: true,
	}
}

func NewLogger(loggerCfg *LoggerCfg) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	dir := filepath.Dir(loggerCfg.LogFilePath)

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		fmt.Println("Failed to create logs directory")
		return nil, err
	}

	if _, err := os.Stat(loggerCfg.LogFilePath); os.IsNotExist(err) {
		_, err = os.Create(loggerCfg.LogFilePath)
		if err != nil {
			fmt.Println("Failed to create log file")
			return nil, err
		}
	}

	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{loggerCfg.LogFilePath}
	cfg.ErrorOutputPaths = []string{loggerCfg.LogFilePath}
	if !loggerCfg.LoggingEnabled {
		cfg.Level = zap.NewAtomicLevelAt(zap.PanicLevel)
	} else {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	if loggerCfg.Stdout {
		cfg.OutputPaths = append(cfg.OutputPaths, "stdout")
		cfg.ErrorOutputPaths = append(cfg.ErrorOutputPaths, "stdout")
	}

	return cfg.Build()
}
