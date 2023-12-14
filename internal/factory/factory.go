package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	dynamicsettings "github.com/denizumutdereli/stream-services/internal/dynamic_settings"
	"github.com/denizumutdereli/stream-services/internal/exchange"

	books "github.com/denizumutdereli/stream-services/internal/exchange/books"
	"github.com/denizumutdereli/stream-services/internal/service"
	"github.com/denizumutdereli/stream-services/internal/service/assets"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"github.com/denizumutdereli/stream-services/internal/types"
	"go.uber.org/zap"
)

type ServiceFactory interface {
	NewAssetsService() (*assets.AssetsService, error)
	NewKafkaManager() (*transport.KafkaManager, error)
	NewWsClient() *transport.WSClient
	NewNatsManager() (*transport.NatsManager, error)
	NewRedisManager() (*transport.RedisManager, error)
	NewStreamService(ctx context.Context, serviceType string) (service.StreamService, error)
}

type serviceFactory struct {
	config          *config.Config
	logger          *zap.Logger
	assets          []string
	nats            *transport.NatsManager
	redis           *transport.RedisManager
	assetsManager   *assets.AssetsService
	settingsManager *dynamicsettings.DynamicsettingsManager
	appContext      *types.ExchangeConfig
	// kafka     *transport.KafkaManager
}

func (f *serviceFactory) NewKafkaManager() (*transport.KafkaManager, error) {
	return transport.NewKafkaManager(f.config.KafkaBrokers, f.config.KafkaConsumerGroup, f.config.MaxRetry, time.Duration(f.config.MaxRetry))
}

func (f *serviceFactory) NewWsClient() *transport.WSClient {
	return transport.NewWSClient(f.config.WsServerURL, time.Duration(f.config.WsPingPeriod), f.config.WsPinMaxError, f.config.MaxRetry, time.Duration(f.config.MaxRetry), f.logger)
}

func (f *serviceFactory) NewNatsManager() (*transport.NatsManager, error) {
	return transport.NewNatsManager(f.config.NatsURL, f.logger)
}

func (f *serviceFactory) NewRedisManager() (*transport.RedisManager, error) {
	return transport.NewRedisManager(f.config.RedisURL, f.config)
}

func (f *serviceFactory) NewSettingsManager() (*dynamicsettings.DynamicsettingsManager, error) {
	return dynamicsettings.NewDynamicsettingsManager(f.config, f.logger, f.redis)
}

func (f *serviceFactory) NewAssetsService() (*assets.AssetsService, error) {
	return assets.NewAssetsService(f.config, f.logger, f.redis)
}

func (f *serviceFactory) NewStreamService(ctx context.Context, serviceType string) (service.StreamService, error) {
	var exchangeService interface {
		StreamData(ctx context.Context)
		WSInstance(ctx context.Context) *transport.WSClient
		SetIsLeader(ctx context.Context, isLeader bool)
	}

	switch serviceType {
	case "tickers":
		exchangeService = exchange.NewTickersManager(f.appContext, f.assetsManager, f.settingsManager)
	case "trades":
		exchangeService = exchange.NewTradesManager(f.appContext, f.assetsManager, f.settingsManager)
	case "markets":
		exchangeService = exchange.NewMarketsManager(f.appContext, f.assetsManager, f.settingsManager)
	case "orderbook":
		exchangeService = books.NewOrderbookManager(f.appContext, f.assetsManager, f.settingsManager)
	default:
		return nil, fmt.Errorf("unknown service type: %s", serviceType)
	}

	f.appContext.Websocket = exchangeService.WSInstance(ctx)
	service := service.NewStreamService(f.appContext)

	go exchangeService.StreamData(ctx)

	go func() {
		for leaderStatus := range f.config.IsLeader {
			isInstanceLeader := leaderStatus
			if leaderStatus {
				f.logger.Info("I am now the leader :)", zap.Bool("isLeader", isInstanceLeader))
				exchangeService.SetIsLeader(ctx, true)
			} else {
				exchangeService.SetIsLeader(ctx, false)
				f.logger.Warn("I lost my leadership :(", zap.Bool("isLeader", isInstanceLeader))
			}
		}
	}()

	return service, nil
}

func NewServiceFactory(config *config.Config) (ServiceFactory, error) {
	factory := &serviceFactory{
		config: config,
		logger: config.Logger,
	}

	var err error

	factory.nats, err = factory.NewNatsManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NatsManager: %w", err)
	}

	factory.redis, err = factory.NewRedisManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RedisManager: %w", err)
	}

	factory.settingsManager, err = factory.NewSettingsManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SettingsManager: %w", err)
	}

	factory.assetsManager, err = factory.NewAssetsService()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AssetsService: %w", err)
	}

	factory.assets, err = factory.assetsManager.GetAssets()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial assets: %w", err)
	}

	factory.appContext = &types.ExchangeConfig{
		Assets: factory.assets,
		Nats:   factory.nats,
		Redis:  factory.redis,
		Config: factory.config,
		Logger: factory.logger,
	}

	return factory, nil
}
