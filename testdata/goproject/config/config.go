package config

// AppConfig holds application configuration.
type AppConfig struct {
	Port    int
	Debug   bool
	DBPath  string
}

// Load reads configuration from environment.
func Load() AppConfig {
	return AppConfig{Port: 8080}
}
