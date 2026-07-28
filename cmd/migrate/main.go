// Command migrate chạy database migration bằng golang-migrate.
//
// Migration là BƯỚC RIÊNG, không chạy trong cmd/api (tránh race khi scale nhiều
// pod). Dùng:
//
//	migrate -config configs/config.local.yaml up
//	migrate -config configs/config.local.yaml down 1
//	migrate -config configs/config.local.yaml force 1
//	migrate -config configs/config.local.yaml version
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/quangdung393/docs-hub-api/internal/config"
)

const migrationsPath = "file://migrations"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate lỗi: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.local.yaml", "đường dẫn file cấu hình")
	flag.Parse()

	cmd := flag.Arg(0)
	if cmd == "" {
		return errors.New("thiếu lệnh: up | down | force | version")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("nạp cấu hình: %w", err)
	}

	m, err := migrate.New(migrationsPath, cfg.MySQL.MigrationDSN())
	if err != nil {
		return fmt.Errorf("khởi tạo migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	return exec(m, cmd, flag.Args())
}

func exec(m *migrate.Migrate, cmd string, args []string) error {
	switch cmd {
	case "up":
		return report(m.Up())
	case "down":
		steps := 1
		if len(args) > 1 {
			n, convErr := strconv.Atoi(args[1])
			if convErr != nil {
				return fmt.Errorf("số bước down không hợp lệ: %w", convErr)
			}
			steps = n
		}
		return report(m.Steps(-steps))
	case "force":
		if len(args) < 2 {
			return errors.New("force cần version: force <v>")
		}
		v, convErr := strconv.Atoi(args[1])
		if convErr != nil {
			return fmt.Errorf("version không hợp lệ: %w", convErr)
		}
		return m.Force(v)
	case "version":
		v, dirty, verErr := m.Version()
		if verErr != nil {
			return fmt.Errorf("đọc version: %w", verErr)
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		return nil
	default:
		return fmt.Errorf("lệnh không hỗ trợ: %q", cmd)
	}
}

// report bỏ qua ErrNoChange (không có gì để migrate là bình thường).
func report(err error) error {
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	fmt.Println("migrate: thành công")
	return nil
}
