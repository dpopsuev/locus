package config

// AppConfig is a configuration struct with struct tags — triggers config_schema anchor.
type AppConfig struct {
	Host     string `json:"host" yaml:"host" env:"APP_HOST"`
	Port     int    `json:"port" yaml:"port" env:"APP_PORT"`
	LogLevel string `json:"log_level" yaml:"log_level" env:"LOG_LEVEL"`
	Debug    bool   `json:"debug" yaml:"debug" env:"DEBUG"`
}
