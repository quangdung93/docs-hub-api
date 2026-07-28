// Package mysql cấu hình kết nối MySQL qua GORM và cung cấp các tiện ích
// transaction/context để tầng repository dùng, giữ cho usecase không biết GORM.
package mysql

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

// Config là tham số kết nối (do caller ánh xạ từ config.MySQLConfig sang, để
// package hạ tầng không phụ thuộc ngược lên package config).
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	SlowThreshold   time.Duration
	LogLevel        logger.LogLevel
}

// New mở kết nối GORM tới MySQL, cấu hình pool và gắn plugin OpenTelemetry.
func New(cfg Config, zlog *zap.Logger) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger:                 newGormLogger(zlog, cfg.SlowThreshold, cfg.LogLevel),
		SkipDefaultTransaction: true, // tự quản transaction, tăng hiệu năng
		PrepareStmt:            true, // cache prepared statement
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("mở kết nối MySQL thất bại: %w", err)
	}

	// Gắn tracing: mỗi query tạo span con.
	if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		return nil, fmt.Errorf("gắn plugin tracing GORM thất bại: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("lấy *sql.DB thất bại: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return db, nil
}

// Close đóng connection pool.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("lấy *sql.DB để đóng thất bại: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("đóng pool MySQL thất bại: %w", err)
	}
	return nil
}
