package config

import (
	"time"
)

type (
	Config struct {
		App       App       `env-prefix:"APP_"`
		Service   Service   `env-prefix:"SERVICE_"`
		Database  Database  `env-prefix:"DB_"`
		Cache     Cache     `env-prefix:"CACHE_"`
		Publisher Publisher `env-prefix:"RABBIT_"`
		SMTP      SMTP      `env-prefix:"SMTP_"`
		TG        TG        `env-prefix:"TG_"`
		HTTP      HTTP      `env-prefix:"HTTP_"`
		Logger    Logger    `env-prefix:"LOGGER_"`
		Env       string    `                      env:"ENV" env-default:"local" validate:"required,oneof=local dev staging prod"`
	}

	App struct {
		Name    string `env:"NAME"    env-default:"delayed-notifier" validate:"required"`
		Version string `env:"VERSION" env-default:"1.0.0"            validate:"required"`
	}

	Service struct {
		QueryLimit uint64        `env:"QUERY_LIMIT" env-default:"10" validate:"min=1,max=100"`
		RetryDelay time.Duration `env:"RETRY_DELAY" env-default:"5m" validate:"gte=1m,lte=1h"`
		MaxRetries int           `env:"MAX_RETRIES" env-default:"3"  validate:"min=1,max=10"`
	}

	Database struct {
		DSN            string        `env:"DSN"              env-default:"postgres://user:pass@localhost:5432/delayed_notifier?sslmode=disable" validate:"required"`
		PoolMax        int32         `env:"POOL_MAX"         env-default:"20"                                                                   validate:"min=1,max=100"`
		ConnAttempts   int           `env:"CONN_ATTEMPTS"    env-default:"5"                                                                    validate:"min=1,max=10"`
		BaseRetryDelay time.Duration `env:"BASE_RETRY_DELAY" env-default:"100ms"                                                                validate:"gte=10ms,lte=10s"`
		MaxRetryDelay  time.Duration `env:"MAX_RETRY_DELAY"  env-default:"5s"                                                                   validate:"gte=100ms,lte=30s,gtefield=BaseRetryDelay"`
	}

	Cache struct {
		Addr     string `env:"ADDR"     env-default:"localhost:6379" validate:"required"`
		Password string `env:"PASSWORD" env-default:""`
	}

	Publisher struct {
		URL            string        `env:"URL"             validate:"required"`
		ConnectionName string        `env:"CONNECTION_NAME"                           env-default:"delayed-notifier-publisher"`
		ConnectTimeout time.Duration `env:"CONNECT_TIMEOUT" validate:"gte=1s,lte=60s" env-default:"30s"`
		Heartbeat      time.Duration `env:"HEARTBEAT"       validate:"gte=1s,lte=60s" env-default:"10s"`
		Exchange       string        `env:"EXCHANGE"        validate:"required"       env-default:"notifications"`
		ContentType    string        `env:"CONTENT_TYPE"                              env-default:"application/json"`

		Attempts int           `env:"ATTEMPTS" env-default:"3"   validate:"min=1,max=10"`
		Delay    time.Duration `env:"DELAY"    env-default:"1s"  validate:"gte=10ms,lte=5m"`
		Backoff  float64       `env:"BACKOFF"  env-default:"2.0" validate:"gte=1.0,lte=5.0"`
	}

	SMTP struct {
		Host     string `env:"HOST"     env-default:"smtp.gmail.com"`
		Port     int    `env:"PORT"     env-default:"587"                 validate:"gte=1,lte=65535"`
		Username string `env:"USERNAME" env-default:""`
		Password string `env:"PASSWORD" env-default:""`
		From     string `env:"FROM"     env-default:"noreply@example.com" validate:"email"`
	}

	TG struct {
		Token string `env:"TOKEN"`
	}

	HTTP struct {
		Host              string        `env:"HOST"                env-default:"0.0.0.0" validate:"required"`
		Port              string        `env:"PORT"                env-default:"8080"    validate:"required"`
		ReadTimeout       time.Duration `env:"READ_TIMEOUT"        env-default:"5s"      validate:"gte=1s,lte=30s"`
		WriteTimeout      time.Duration `env:"WRITE_TIMEOUT"       env-default:"5s"      validate:"gte=1s,lte=30s"`
		IdleTimeout       time.Duration `env:"IDLE_TIMEOUT"        env-default:"60s"     validate:"gte=1s,lte=300s"`
		ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT"    env-default:"10s"     validate:"gte=1s,lte=30s"`
		ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" env-default:"5s"      validate:"gte=1s,lte=30s"`
		MaxHeaderBytes    int           `env:"MAX_HEADER_BYTES"    env-default:"1048576" validate:"required,gte=1024,lte=10485760"`
	}

	Logger struct {
		Level      string `env:"LEVEL"       env-default:"info"                        validate:"oneof=debug info warn error"`
		Filename   string `env:"FILENAME"    env-default:"./logs/delayed-notifier.log"`
		MaxSize    int    `env:"MAX_SIZE"    env-default:"100"                         validate:"min=1,max=1000"`
		MaxBackups int    `env:"MAX_BACKUPS" env-default:"3"                           validate:"min=0,max=20"`
		MaxAge     int    `env:"MAX_AGE"     env-default:"28"                          validate:"min=1,max=365"`
	}
)
