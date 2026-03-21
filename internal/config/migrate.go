package config

// MigrateConfig applies any necessary migrations to bring the config up to date.
// For v1, this is a no-op but establishes the pattern for future migrations.
func MigrateConfig(c *Config) (*Config, error) {
	return c, nil
}
