// InterceptorLogger adapts zap logger to interceptor logger.
package mycli

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"

	"go.uber.org/zap"
)

// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			case proto.Message:
				b, err := protojson.Marshal(v)
				if err != nil {
					// Associate the error with the original key to avoid collisions
					// and to make it clear which field failed to marshal.
					f = append(f, zap.String(key.(string), fmt.Sprintf("ERROR: failed to marshal proto type %s: %v", string(v.ProtoReflect().Descriptor().FullName()), err)))
				} else {
					f = append(f, zap.Any(key.(string), json.RawMessage(b)))
				}
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			// Log as error for unknown levels instead of panicking
			logger.Error(fmt.Sprintf("unknown log level %v: %s", lvl, msg))
		}
	})
}
