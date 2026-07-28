package postgres

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	applogger "github.com/quangdung93/docs-hub-api/pkg/logger"
)

// gormZapLogger nối GORM logger vào Zap, và tự lấy logger theo request từ context
// (đã kèm request_id/trace_id nếu có).
type gormZapLogger struct {
	base          *zap.Logger
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(base *zap.Logger, slow time.Duration, level gormlogger.LogLevel) gormlogger.Interface {
	if slow <= 0 {
		slow = 200 * time.Millisecond
	}
	if level == 0 {
		level = gormlogger.Warn
	}
	return &gormZapLogger{base: base, level: level, slowThreshold: slow}
}

func (l *gormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *gormZapLogger) logger(ctx context.Context) *zap.Logger {
	if fromCtx := applogger.FromContext(ctx); fromCtx != nil {
		return fromCtx
	}
	return l.base
}

func (l *gormZapLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Info {
		l.logger(ctx).Sugar().Infof(msg, data...)
	}
}

func (l *gormZapLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Warn {
		l.logger(ctx).Sugar().Warnf(msg, data...)
	}
}

func (l *gormZapLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Error {
		l.logger(ctx).Sugar().Errorf(msg, data...)
	}
}

func (l *gormZapLogger) Trace(
	ctx context.Context, begin time.Time,
	fc func() (sql string, rowsAffected int64), err error,
) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}

	log := l.logger(ctx)
	switch {
	case err != nil && !errors.Is(err, gormlogger.ErrRecordNotFound) && l.level >= gormlogger.Error:
		log.Error("gorm query lỗi", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		log.Warn("gorm query chậm", fields...)
	case l.level >= gormlogger.Info:
		log.Debug("gorm query", fields...)
	}
}
