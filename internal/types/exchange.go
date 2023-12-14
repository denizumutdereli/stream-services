package types

import (
	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"go.uber.org/zap"
)

type ExchangeConfig struct {
	Assets    []string
	Nats      *transport.NatsManager
	Redis     *transport.RedisManager
	Websocket *transport.WSClient
	Kafka     *transport.KafkaManager
	Config    *config.Config
	Logger    *zap.Logger
	Service   *interface{}
}
