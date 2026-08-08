// Package logging creates the process-wide structured zap logger.
package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a console development logger or JSON production logger.
func New(environment, level string) (*zap.Logger, error) {
	var cfg zap.Config
	if environment == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	}

	if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}
