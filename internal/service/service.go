package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"github.com/denizumutdereli/stream-services/internal/types"
	"go.uber.org/zap"
)

type ServiceStatuses struct {
	redisConnected     bool
	websocketConnected bool
	natsConnected      bool
}

type StreamService interface {
	MonitorServices(ctx context.Context)
	Live(ctx context.Context) (int, bool, error)
	Read(ctx context.Context) (int, bool, error)
	IsLeader(ctx context.Context, isLeader bool)
}

type streamService struct {
	config *config.Config
	logger *zap.Logger
	assets []string
	// markup    *markup.MarkupMarkDownManager
	kafka     *transport.KafkaManager
	redis     *transport.RedisManager
	webSocket *transport.WSClient
	nats      *transport.NatsManager
	status    *ServiceStatuses
	isLeader  bool
}

func NewStreamService(appContext *types.ExchangeConfig) StreamService {
	service := &streamService{
		assets:    appContext.Assets,
		kafka:     appContext.Kafka,
		redis:     appContext.Redis,
		webSocket: appContext.Websocket,
		nats:      appContext.Nats,
		config:    appContext.Config,
		logger:    appContext.Logger,
		status:    &ServiceStatuses{redisConnected: true, websocketConnected: true, natsConnected: true},
		// markup:    mkp,
		isLeader: false,
	}

	ctx := context.Background()

	go service.MonitorServices(ctx)

	return service
}

func (s *streamService) IsLeader(ctx context.Context, isLeader bool) {
	s.isLeader = isLeader
}

func (s *streamService) MonitorServices(ctx context.Context) {
	s.logger.Debug("## Service monitoring started")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			s.status.redisConnected = s.redis.IsConnected()
			s.status.websocketConnected = s.webSocket.IsConnected()
			s.status.natsConnected = s.nats.IsConnected()

			if !(s.status.redisConnected && s.status.websocketConnected && s.status.natsConnected) {
				s.logger.Debug("service status:", zap.Bool("Redis", s.status.redisConnected), zap.Bool("Websocket", s.status.websocketConnected),
					zap.Bool("Nats", s.status.natsConnected))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *streamService) Live(ctx context.Context) (int, bool, error) {
	serviceStatus := fmt.Sprintf("Redis:%v WebSocket: %v, Nats:%v", s.status.redisConnected, s.status.websocketConnected, s.status.natsConnected)
	if s.status.redisConnected && s.status.websocketConnected && s.status.natsConnected {
		return http.StatusOK, true, nil
	}
	return http.StatusServiceUnavailable, false, fmt.Errorf("services are not fully operational %s", serviceStatus)
}

func (s *streamService) Read(ctx context.Context) (int, bool, error) {
	return s.Live(ctx)
}
