package config

import "fmt"

// DSN dựng chuỗi kết nối PostgreSQL cho GORM driver (dạng key-value).
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		p.Host, p.Port, p.User, p.Password, p.Database, p.SSLMode)
}

// MigrationDSN dựng URL cho golang-migrate (scheme postgres://).
func (p PostgresConfig) MigrationDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.Database, p.SSLMode)
}

// Addr trả về host:port của Redis.
func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// URL dựng AMQP URL cho RabbitMQ.
func (r RabbitMQConfig) URL() string {
	vhost := r.VHost
	if vhost == "/" {
		vhost = "" // amqp://host:port/ nghĩa là vhost mặc định "/"
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", r.User, r.Password, r.Host, r.Port, vhost)
}
