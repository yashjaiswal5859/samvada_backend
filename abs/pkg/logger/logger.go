// =============================================================================
// Samvada Platform — Enterprise Structured Logger (Zap)
// =============================================================================

package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type correlationKeyType struct{}

var correlationKey = correlationKeyType{}

// GlobalLogger is the package-level default logger instance.
var GlobalLogger *zap.Logger

// Init initializes the enterprise Zap logger with environment awareness.
func Init(env string, levelStr string) *zap.Logger {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = zapcore.InfoLevel
	}

	var encoderConfig zapcore.EncoderConfig
	var encoder zapcore.Encoder

	if env == "production" {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		level,
	)

	GlobalLogger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	zap.ReplaceGlobals(GlobalLogger)

	return GlobalLogger
}

// WithCorrelationID stores a request/correlation ID in context.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationKey, correlationID)
}

// FromContext extracts the correlation ID from context and appends it to the logger.
func FromContext(ctx context.Context) *zap.Logger {
	if GlobalLogger == nil {
		GlobalLogger = zap.L()
	}
	if ctx == nil {
		return GlobalLogger
	}
	if cid, ok := ctx.Value(correlationKey).(string); ok && cid != "" {
		return GlobalLogger.With(zap.String("correlation_id", cid))
	}
	return GlobalLogger
}
