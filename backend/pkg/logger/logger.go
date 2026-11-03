// Package logger 提供基于 Zap 的结构化日志初始化与便捷方法。
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Init 初始化全局 Zap 日志。debug 模式输出彩色日志，release 模式输出 JSON。
//
// debug 的取值与 config.ModeDebug 同值（"debug"）：archtest 强制的是反向边——
// config 不 import pkg；logger 保持不依赖 config，因此以字面量判定，并以注释与
// config.ModeDebug 双向点名——改任一侧的取值要同步改另一侧。
func Init(mode string) error {
	var config zap.Config

	if mode == "debug" { // == config.ModeDebug（见上：logger 不依赖 config）
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
