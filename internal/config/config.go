package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/ilyakaznacheev/cleanenv"
)

type (
	Config struct {
		App     App     `env-prefix:"APP_"`
		Logger  Logger  `env-prefix:"LOGGER_"`
		Redis   Redis   `env-prefix:"REDIS_"`
		Email   Email   `env-prefix:"EMAIL_"`
		Metrics Metrics `env-prefix:"METRICS_"`
		Env     string  `env:"ENV" env-default:"local" validate:"oneof=local dev staging prod"`
	}

	App struct {
		Port    int    `env:"PORT"    validate:"gte=1,lte=65535" env-default:"8080"`
		Name    string `env:"NAME"    validate:"required"`
		Version string `env:"VERSION" validate:"required"`
	}

	HTTP struct {
		Host              string        `env:"HOST"                validate:"required"                 env-default:"0.0.0.0"`
		Port              string        `env:"PORT"                validate:"required,gte=1,lte=65535" env-default:"8080"`
		ReadTimeout       time.Duration `env:"READ_TIMEOUT"        validate:"gte=10ms,lte=30s"         env-default:"5s"`
		WriteTimeout      time.Duration `env:"WRITE_TIMEOUT"       validate:"gte=10ms,lte=30s"         env-default:"5s"`
		IdleTimeout       time.Duration `env:"IDLE_TIMEOUT"        validate:"gte=10ms,lte=30s"         env-default:"60s"`
		ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT"    validate:"gte=10ms,lte=30s"         env-default:"10s"`
		ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" validate:"gte=10ms,lte=30s"         env-default:"5s"`
	}

	Redis struct {
		Addr        string        `env:"ADDR" validate:"required"`
		Password    string        `env:"PASSWORD" validate:"required"`
		PoolSize    int           `env:"POOL_SIZE" validate:"min=1,max=100" env-default:"20"`
		MinIdleCons int           `env:"MIN_IDLE_CONS" validate:"min=1,max=100" env-default:"10"`
		PoolTimeout time.Duration `env:"POOL_TIMEOUT" validate:"gte=10ms,lte=10s" env-default:"100ms"`
	}

	Email struct {
		Host     string `env:"HOST,required" validate:"required"                 env-default:"0.0.0.0"`
		Port     int    `env:"PORT" validate:"required,gte=1,lte=65535" env-default:"8080"`
		Username string `env:"USERNAME" validate:"required" env-default:"admin"`
		Password string `env:"PASSWORD" validate:"required" env-default:"admin"`
		Sender   string `env:"SENDER" validate:"required"`
	}

	Metrics struct {
		Host              string        `env:"HOST"                validate:"required"                 env-default:"0.0.0.0"`
		Port              string        `env:"PORT"                validate:"required,gte=1,lte=65535" env-default:"8081"`
		ReadTimeout       time.Duration `env:"READ_TIMEOUT"        validate:"gte=10ms,lte=30s"         env-default:"5s"`
		WriteTimeout      time.Duration `env:"WRITE_TIMEOUT"       validate:"gte=10ms,lte=30s"         env-default:"5s"`
		ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" validate:"gte=10ms,lte=30s"         env-default:"5s"`
	}

	Logger struct {
		Level      string `env:"LEVEL"       env-default:"info"                     validate:"oneof=debug info warn error"`
		Filename   string `env:"FILENAME"    env-default:"./logs/order-service.log"`
		MaxSize    int    `env:"MAX_SIZE"    env-default:"100"                      validate:"min=1,max=1000"`
		MaxBackups int    `env:"MAX_BACKUPS" env-default:"3"                        validate:"min=0,max=20"`
		MaxAge     int    `env:"MAX_AGE"     env-default:"28"                       validate:"min=1,max=365"`
	}
)

func Load() (*Config, error) {
	path := fetchConfigPath()
	if path == "" {
		return nil, entity.ErrConfigPathNotSet
	}
	return LoadPath(path)
}

func LoadPath(configPath string) (*Config, error) {
	const op = "config.LoadPath"

	validate := validator.New()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: config file does not exist: %s", op, configPath)
	} else if err != nil {
		return nil, fmt.Errorf("%s: checking config file: %w", op, err)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("%s: read config: %w", op, err)
	}

	var validationErrors []string
	if err := validate.Struct(&cfg); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			for _, ve := range validationErrs {
				validationErrors = append(validationErrors,
					fmt.Sprintf("%s=%v must satisfy '%s'", ve.Field(), ve.Value(), ve.Tag()))
			}
			return nil, fmt.Errorf(
				"%s: config validation: %v", op,
				strings.Join(validationErrors, "; "),
			)
		}
		return nil, fmt.Errorf("%s: config validation: %w", op, err)
	}

	return &cfg, nil
}

func fetchConfigPath() string {
	var path string
	flag.StringVar(&path, "config", "", "Path to config file")
	flag.Parse()

	if path == "" {
		path = os.Getenv("CONFIG_PATH")
	}
	return path
}
