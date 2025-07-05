package main

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const serverTimingHeaderKey = "server-timing"

// serverTimingUnaryInterceptor creates a UnaryClientInterceptor that logs server-timing headers
func serverTimingUnaryInterceptor(logger *zap.Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		
		var header metadata.MD
		opts = append(opts, grpc.Header(&header))
		
		err := invoker(ctx, method, req, reply, cc, opts...)
		
		// Log server-timing header if present
		if serverTimingValues := header.Get(serverTimingHeaderKey); len(serverTimingValues) > 0 {
			logger.Debug("server-timing header (unary)",
				zap.String("method", method),
				zap.Strings("values", serverTimingValues))
		}
		
		return err
	}
}

// serverTimingStreamInterceptor creates a StreamClientInterceptor that logs server-timing headers
func serverTimingStreamInterceptor(logger *zap.Logger) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		
		// Create a wrapper stream to intercept headers
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		
		return &serverTimingStream{
			ClientStream: clientStream,
			method:       method,
			logger:       logger,
		}, nil
	}
}

// serverTimingStream wraps a ClientStream to log server-timing headers
type serverTimingStream struct {
	grpc.ClientStream
	method string
	logger *zap.Logger
	logged bool
}

// Header overrides the Header method to log server-timing values
func (s *serverTimingStream) Header() (metadata.MD, error) {
	header, err := s.ClientStream.Header()
	
	// Only log once per stream
	if !s.logged && err == nil {
		if serverTimingValues := header.Get(serverTimingHeaderKey); len(serverTimingValues) > 0 {
			s.logger.Debug("server-timing header (stream)",
				zap.String("method", s.method),
				zap.Strings("values", serverTimingValues))
		}
		s.logged = true
	}
	
	return header, err
}