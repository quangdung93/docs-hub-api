//go:build integration

package ingestion

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Integration test của ingestion cần schema THẬT: 8 bảng ràng buộc khoá ngoại
// với nhau, cột có DEFAULT mà chính mã nguồn dựa vào (max_attempts DEFAULT 3,
// available_at DEFAULT now()). Không dựng nổi bằng vài câu CREATE TABLE tối giản
// như test của auth/user/project đang làm.
//
// Nhưng KHÔNG được chạy migration vào database dùng chung: ba bộ test kia tự tạo
// bảng tối giản với `IF NOT EXISTS` và giả định database sạch. Chạy migration
// trước thì `IF NOT EXISTS` bỏ qua, rồi INSERT của chúng vi phạm NOT NULL —
// đã thử và làm đỏ TestStats_*.
//
// Nên bộ này dùng database RIÊNG, tự tạo và tự migrate. Hai bên không biết tới
// nhau, không cần sửa CI cũng không cần sửa test của module khác.
const (
	testDatabase  = "document_hub_ingestion_it"
	migrationsDir = "../../../migrations"
)

var (
	prepareOnce sync.Once
	preparedDSN string
	prepareErr  error
)

// parseDSN tách chuỗi dạng `key=value key=value`. Đủ dùng cho DSN của CI và của
// máy dev; không xử lý giá trị có dấu cách hay dấu nháy.
func parseDSN(dsn string) map[string]string {
	out := map[string]string{}
	for _, phan := range strings.Fields(dsn) {
		if k, v, ok := strings.Cut(phan, "="); ok {
			out[k] = v
		}
	}
	return out
}

func buildDSN(fields map[string]string) string {
	parts := make([]string, 0, len(fields))
	for _, k := range []string{"host", "port", "user", "password", "dbname", "sslmode", "TimeZone"} {
		if v, ok := fields[k]; ok {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

// buildMigrateURL đổi sang dạng URL vì golang-migrate không nhận dạng key=value.
func buildMigrateURL(fields map[string]string) string {
	sslMode := fields["sslmode"]
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		fields["user"], fields["password"], fields["host"], fields["port"],
		fields["dbname"], sslMode)
}

// prepareTestDatabase tạo database riêng rồi chạy migration thật vào đó. Chỉ
// làm một lần cho cả package; CI chạy `-p 1` nên không có hai package cùng lúc.
func prepareTestDatabase(baseDSN string) (string, error) {
	prepareOnce.Do(func() {
		admin, err := gorm.Open(gormpg.Open(baseDSN), &gorm.Config{})
		if err != nil {
			prepareErr = fmt.Errorf("mở database gốc: %w", err)
			return
		}
		defer func() {
			if sqlDB, dbErr := admin.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
		}()

		var count int64
		if err = admin.Raw(`SELECT count(*) FROM pg_database WHERE datname=?`,
			testDatabase).Scan(&count).Error; err != nil {
			prepareErr = fmt.Errorf("tra database %s: %w", testDatabase, err)
			return
		}
		if count == 0 {
			// CREATE DATABASE không nhận tham số bind và không chạy được trong
			// transaction. Tên là hằng khai ngay trên nên không có đường tiêm SQL.
			if err = admin.Exec(`CREATE DATABASE ` + testDatabase).Error; err != nil {
				prepareErr = fmt.Errorf("tạo database %s: %w", testDatabase, err)
				return
			}
		}

		fields := parseDSN(baseDSN)
		fields["dbname"] = testDatabase
		preparedDSN = buildDSN(fields)

		m, err := migrate.New("file://"+migrationsDir, buildMigrateURL(fields))
		if err != nil {
			prepareErr = fmt.Errorf("khởi tạo migrate: %w", err)
			return
		}
		defer func() { _, _ = m.Close() }()
		if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			prepareErr = fmt.Errorf("chạy migration: %w", err)
		}
	})
	return preparedDSN, prepareErr
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := os.Getenv("TEST_POSTGRES_DSN")
	if baseDSN == "" {
		t.Skip("cần TEST_POSTGRES_DSN để chạy test này")
	}
	dsn, err := prepareTestDatabase(baseDSN)
	require.NoError(t, err)
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}
