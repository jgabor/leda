package db

import "example.com/testproject/config"

// DB represents a database connection.
type DB struct {
	cfg config.AppConfig
}

// Open creates a new database connection.
func Open() *DB {
	cfg := config.Load()
	return &DB{cfg: cfg}
}
