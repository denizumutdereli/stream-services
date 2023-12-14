package config

import (
	"log"
	"os"

	"github.com/RackSec/srslog"
	"github.com/go-playground/validator"
	"github.com/mattn/go-colorable"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ExchangesConfig struct {
	Binance BinanceConfig `mapstructure:"Binance"`
}

type BinanceConfig struct {
	StreamWSURL           string `mapstructure:"WS_STREAM_URL" validate:"required"`
	RateLimit             int    `mapstructure:"RATE_LIMIT" validate:"required"`
	RestURL               string `mapstructure:"REST_URL" validate:"required"`
	SnapshotChunkSize     int    `mapstructure:"SNAPSHOT_CHUNK_SIZE" validate:"required"`
	SnapshotWaitTime      int    `mapstructure:"SNAPSHOT_WAIT_TIME" validate:"required"`
	SnapshotDepth         int    `mapstructure:"SNAPSHOT_DEPTH" validate:"required"`
	DefaultStreamInterval string `mapstructure:"DEFAULT_STREAM_INTERVAL" validate:"required"`
}

type Config struct {
	AppName               string   `mapstructure:"APP_NAME" validate:"required"`
	AllowedServices       []string `mapstructure:"ALLOWED_SERVICES" validate:"required"`
	AssetsApi             string   `mapstructure:"ASSETS_API"`
	DefaultTickerInterval int      `mapstructure:"DEFAULT_TICKER_INTERVAL" validate:"required"`
	NodeUUID              string
	EtcdUrl               string `mapstructure:"ETCD_URL" validate:"required"`
	EtcdNodes             int
	Exchanges             ExchangesConfig `mapstructure:"EXCHANGES"`
	GoServicePort         string          `mapstructure:"GO_SERVICE_PORT" validate:"required"`
	Https                 string          `mapstructure:"HTTPS" validate:"required"`
	IgnoredAsssets        []string        `mapstructure:"IGNORED_ASSETS"`
	IsLeader              chan bool
	KafkaBrokers          []string `mapstructure:"KAFKA_BROKERS" validate:"required"`
	KafkaConsumerGroup    string   `mapstructure:"KAFKA_CONSUMER_GROUP" validate:"required"`
	KafkaConsumeTopics    []string `mapstructure:"KAFKA_CONSUME_TOPICS" validate:"required"`
	KafkaProduceTopic     string   `mapstructure:"KAFKA_PRODUCE_TOPIC" validate:"required"`
	Logger                *zap.Logger
	LoggerSys             *srslog.Writer
	MaxAppErrors          int      `mapstructure:"MAX_APP_ERRORS"`
	MaxRetry              int      `mapstructure:"MAX_RETRY"`
	MaxSpreadPct          float64  `mapstructure:"MAX_SPREAD_PCT"`
	MaxWait               int      `mapstructure:"MAX_WAIT"`
	NatsURL               []string `mapstructure:"NATS_URL" validate:"required"`
	RedisURL              string   `mapstructure:"REDIS_URL"`
	ServiceName           string
	SettingsApi           string `mapstructure:"SETTINGS_API" validate:"required"`
	SysLog                string `mapstructure:"SYSLOG" validate:"required"`
	Test                  string `mapstructure:"TEST" validate:"required"`
	WsPinMaxError         int    `mapstructure:"WSPING_MAX_ERROR"`
	WsPingPeriod          int    `mapstructure:"WSPING_PERIOD"`
	WsServerURL           string `mapstructure:"WSSERVER_URL"`
}

var config = &Config{}

func init() {
	viper.SetConfigName("config")
	viper.AddConfigPath("./internal/config/")
	viper.SetConfigType("json")

	viper.SetDefault("REDIS_PORT", 6379)
	viper.SetDefault("MAX_RETRY", 5)
	viper.SetDefault("MAX_WAIT", 2000)
	viper.SetDefault("MAX_SPREAD_PCT", 35)

	log.Println("Reading config...")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Fatal error config: %v", err)
	}

	log.Println("Unmarshalling config...")
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Fatalf("Unable to decode into struct, %v", err)
	}

	if config.SysLog == "true" {

		config.LoggerSys, err = srslog.Dial("", "", srslog.LOG_INFO, "CEF0")
		if err != nil {
			log.Println("Error setting up syslog:", err)
			os.Exit(1)
		}

	}

	logs := zap.NewDevelopmentEncoderConfig() //zap.NewProduction()
	logs.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.Logger = zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(logs),
		zapcore.AddSync(colorable.NewColorableStdout()),
		zapcore.DebugLevel,
	))
	defer config.Logger.Sync()

	validate := validator.New()
	err = validate.Struct(config)
	if err != nil {
		log.Fatalf("Config validation failed, %v", err)
	}
}

func GetConfig() (*Config, error) {
	return config, nil
}
