package config

import (
	"time"
)

type (
	Config struct {
		App      App      `env-prefix:"APP_"`
		Database Database `env-prefix:"DB_"`
		Redis    Redis    `env-prefix:"REDIS_"`
		Rabbit   Rabbit   `env-prefix:"RABBIT_"`
		SMTP     SMTP     `env-prefix:"SMTP_"`
		TG       TG       `env-prefix:"TG_"`
		HTTP     HTTP     `env-prefix:"HTTP_"`
		Logger   Logger   `env-prefix:"LOGGER_"`
		Env      string   `env:"ENV" env-default:"local" validate:"oneof=local dev staging prod"`
	}

	App struct {
		Port    int    `env:"PORT"    validate:"gte=1,lte=65535" env-default:"8080"`
		Name    string `env:"NAME"    validate:"required"`
		Version string `env:"VERSION" validate:"required"`
	}

	Database struct {
		Host           string        `env:"HOST"             validate:"required"`
		Port           string        `env:"PORT"             validate:"required,gte=1,lte=65535"`
		Name           string        `env:"NAME"             validate:"required"`
		User           string        `env:"USER"             validate:"required"`
		Password       string        `env:"PASSWORD"         validate:"required"`
		SSLMode        string        `env:"SSL_MODE"         validate:"required"`
		PoolMax        int32         `env:"POOL_MAX"         validate:"min=1,max=100"                             env-default:"20"`
		ConnAttempts   int           `env:"CONN_ATTEMPTS"    validate:"min=1,max=10"                              env-default:"5"`
		BaseRetryDelay time.Duration `env:"BASE_RETRY_DELAY" validate:"gte=10ms,lte=10s"                          env-default:"100ms"`
		MaxRetryDelay  time.Duration `env:"MAX_RETRY_DELAY"  validate:"gte=100ms,lte=30s,gtefield=BaseRetryDelay" env-default:"5s"`
	}

	Redis struct {
		Addr        string        `env:"ADDR" validate:"required"`
		Password    string        `env:"PASSWORD" validate:"required"`
		PoolSize    int           `env:"POOL_SIZE" validate:"min=1,max=100" env-default:"20"`
		MinIdleCons int           `env:"MIN_IDLE_CONS" validate:"min=1,max=100" env-default:"10"`
		PoolTimeout time.Duration `env:"POOL_TIMEOUT" validate:"gte=10ms,lte=10s" env-default:"100ms"`
	}

	Rabbit struct {
		GroupID  string `env:"URL" validate:"required"`
		Exchange string `env:"EXCHANGE"  validate:"required"`
		Queue    string `env:"QUEUE"    validate:"required"`
	}

	SMTP struct {
		Host     string `env:"HOST"             validate:"required"`
		Port     int    `env:"PORT"             validate:"required,gte=1,lte=65535"`
		Username string `env:"USERNAME"             validate:"required"`
		Password string `env:"PASSWORD"             validate:"required"`
	}

	TG struct {
		Token string `env:"TOKEN"             validate:"required"`
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

	Logger struct {
		Level      string `env:"LEVEL"       env-default:"info"                     validate:"oneof=debug info warn error"`
		Filename   string `env:"FILENAME"    env-default:"./logs/order-service.log"`
		MaxSize    int    `env:"MAX_SIZE"    env-default:"100"                      validate:"min=1,max=1000"`
		MaxBackups int    `env:"MAX_BACKUPS" env-default:"3"                        validate:"min=0,max=20"`
		MaxAge     int    `env:"MAX_AGE"     env-default:"28"                       validate:"min=1,max=365"`
	}
)
