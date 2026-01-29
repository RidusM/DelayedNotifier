package config

import (
	"time"

	"github.com/wb-go/wbf/retry"
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
		Env       string    `                      env:"ENV" env-default:"local" validate:"oneof=local dev staging prod"`
	}

	App struct {
		Port    int    `env:"PORT"    env-default:"8080"             validate:"gte=1,lte=65535"`
		Name    string `env:"NAME"    env-default:"delayed-notifier" validate:"required"`
		Version string `env:"VERSION" env-default:"1.0.0"            validate:"required"`
	}

	Service struct {
		QueryLimit uint64        `env:"QUERY_LIMIT" env-default:"10" validate:"min=1,max=100"`
		RetryDelay time.Duration `env:"RETRY_DELAY" env-default:"5m" validate:"gte=1m,lte=1h"`
		MaxRetries uint32        `env:"MAX_RETRIES" env-default:"3"  validate:"min=1,max=10"`
	}

	Database struct {
		DSN            string        `env:"DSN"              env-default:"postgres://user:pass@localhost:5432/delayed_notifier?sslmode=disable" validate:"required"`
		PoolMax        int32         `env:"POOL_MAX"         env-default:"20"                                                                   validate:"min=1,max=100"`
		ConnAttempts   int           `env:"CONN_ATTEMPTS"    env-default:"5"                                                                    validate:"min=1,max=10"`
		BaseRetryDelay time.Duration `env:"BASE_RETRY_DELAY" env-default:"100ms"                                                                validate:"gte=10ms,lte=10s"`
		MaxRetryDelay  time.Duration `env:"MAX_RETRY_DELAY"  env-default:"5s"                                                                   validate:"gte=100ms,lte=30s,gtefield=BaseRetryDelay"`
	}

	Cache struct {
		Addr        string        `env:"ADDR"          env-default:"localhost:6379" validate:"required"`
		Password    string        `env:"PASSWORD"      env-default:""`
		PoolSize    int           `env:"POOL_SIZE"     env-default:"20"             validate:"min=1,max=100"`
		MinIdleCons int           `env:"MIN_IDLE_CONS" env-default:"10"             validate:"min=1,max=100"`
		PoolTimeout time.Duration `env:"POOL_TIMEOUT"  env-default:"4s"             validate:"gte=10ms,lte=10s"`
	}

	Publisher struct {
		URL            string        `env:"URL"             env-default:"amqp://guest:guest@localhost:5672/" validate:"required"`
		ConnectionName string        `env:"CONNECTION_NAME" env-default:"delayed-notifier-publisher"         validate:"required"`
		ConnectTimeout time.Duration `env:"CONNECT_TIMEOUT" env-default:"30s"                                validate:"gte=1s,lte=60s"`
		Heartbeat      time.Duration `env:"HEARTBEAT"       env-default:"10s"                                validate:"gte=1s,lte=60s"`
		Exchange       string        `env:"EXCHANGE"        env-default:"notifications"                      validate:"required"`
		ContentType    string        `env:"CONTENT_TYPE"    env-default:"application/json"                   validate:"required"`
		ReconnectStrat retry.Strategy
		ProducingStrat retry.Strategy
	}

	SMTP struct {
		Host     string `env:"HOST"     env-default:"smtp.gmail.com"      validate:"required"`
		Port     int    `env:"PORT"     env-default:"587"                 validate:"required,gte=1,lte=65535"`
		Username string `env:"USERNAME" env-default:""                    validate:"required"`
		Password string `env:"PASSWORD" env-default:""                    validate:"required"`
		From     string `env:"FROM"     env-default:"noreply@example.com" validate:"required,email"`
	}

	TG struct {
		Token string `env:"TOKEN" env-default:"" validate:"required"`
	}

	HTTP struct {
		Host              string        `env:"HOST"                env-default:"0.0.0.0" validate:"required"`
		Port              string        `env:"PORT"                env-default:"8080"    validate:"required"`
		ReadTimeout       time.Duration `env:"READ_TIMEOUT"        env-default:"5s"      validate:"gte=1s,lte=30s"`
		WriteTimeout      time.Duration `env:"WRITE_TIMEOUT"       env-default:"5s"      validate:"gte=1s,lte=30s"`
		IdleTimeout       time.Duration `env:"IDLE_TIMEOUT"        env-default:"60s"     validate:"gte=1s,lte=300s"`
		ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT"    env-default:"10s"     validate:"gte=1s,lte=30s"`
		ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" env-default:"5s"      validate:"gte=1s,lte=30s"`
	}

	Logger struct {
		Level      string `env:"LEVEL"       env-default:"info"                        validate:"oneof=debug info warn error"`
		Filename   string `env:"FILENAME"    env-default:"./logs/delayed-notifier.log"`
		MaxSize    int    `env:"MAX_SIZE"    env-default:"100"                         validate:"min=1,max=1000"`
		MaxBackups int    `env:"MAX_BACKUPS" env-default:"3"                           validate:"min=0,max=20"`
		MaxAge     int    `env:"MAX_AGE"     env-default:"28"                          validate:"min=1,max=365"`
	}
)
