package config

import "fmt"

// DSN dựng chuỗi kết nối MySQL cho GORM driver.
// Định dạng: user:pass@tcp(host:port)/db?params
func (m MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		m.User, m.Password, m.Host, m.Port, m.Database, m.Params)
}

// MigrationDSN dựng DSN cho golang-migrate (scheme mysql://).
func (m MySQLConfig) MigrationDSN() string {
	return fmt.Sprintf("mysql://%s:%s@tcp(%s:%d)/%s?multiStatements=true",
		m.User, m.Password, m.Host, m.Port, m.Database)
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
