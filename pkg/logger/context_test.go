package logger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"document-hub-api/pkg/logger"
)

func TestFromContext_ReturnsNopWhenAbsent(t *testing.T) {
	// Không panic, không nil dù context rỗng.
	l := logger.FromContext(context.Background())
	require.NotNil(t, l)
	l.Info("an toàn với context rỗng")
}

func TestWithContext_RoundTrip(t *testing.T) {
	base, err := logger.New(logger.Options{Level: "info", Encoding: "json", AppName: "t", Env: "test"})
	require.NoError(t, err)

	child := base.With(zap.String("request_id", "req-123"))
	ctx := logger.WithContext(context.Background(), child)

	require.Same(t, child, logger.FromContext(ctx))
}
