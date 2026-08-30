// Package logger 提供基于 Zap 的结构化日志初始化与便捷方法。
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化全局 Zap 日志。debug 模式输出彩色日志，release 模式输出 JSON。
func Init(mode string) error {
	var config zap.Config

	if mode == "debug" {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	config.OutputPaths = []string{"stdout", "logs/app.log"}
	config.ErrorOutputPaths = []string{"stderr", "logs/error.log"}

	// 确保 logs 目录存在
	os.MkdirAll("logs", 0755)

	logger, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(logger)
	return nil
}

// Info 输出 INFO 级别日志。
func Info(msg string, fields ...zap.Field) {
	zap.L().Info(msg, fields...)
}

// Error 输出 ERROR 级别日志。
func Error(msg string, fields ...zap.Field) {
	zap.L().Error(msg, fields...)
}

// Debug 输出 DEBUG 级别日志。
func Debug(msg string, fields ...zap.Field) {
	zap.L().Debug(msg, fields...)
}

// Warn 输出 WARN 级别日志。
func Warn(msg string, fields ...zap.Field) {
	zap.L().Warn(msg, fields...)
}
