package exchange

/*
{
  "e": "trade",       // Event type
  "E": 1672515782136, // Event time
  "s": "BNBBTC",      // Symbol
  "t": 12345,         // Trade ID
  "p": "0.001",       // Price
  "q": "100",         // Quantity
  "b": 88,            // Buyer order ID
  "a": 50,            // Seller order ID
  "T": 1672515782136, // Trade time
  "m": true,          // Is the buyer the market maker?
  "M": true           // Ignore
}
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	dynamicsettings "github.com/denizumutdereli/stream-services/internal/dynamic_settings"
	"github.com/denizumutdereli/stream-services/internal/service/assets"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"github.com/denizumutdereli/stream-services/internal/types"
	"go.uber.org/zap"
)

type TradesData struct {
	EventType   string `json:"e"`  // Event type
	EventTime   int64  `json:"E"`  // Event time
	Symbol      string `json:"s"`  // Symbol
	Price       string `json:"p"`  // Price
	Quantity    string `json:"q"`  // Quantity
	TradeTime   int64  `json:"T"`  // TradeTime
	MarketMaker bool   `json:"m"`  // Is the buyer the market maker? true=> Sell, false=>Buy
	Side        string `json:"sd"` // Side
}

type TradesManager struct {
	assets          []string
	assetsManager   *assets.AssetsService
	settingsManager *dynamicsettings.DynamicsettingsManager
	config          *config.Config
	isLeader        bool
	logger          *zap.Logger
	maxSpreadPct    float64
	nats            *transport.NatsManager
	streamData      *TradesData
	Websocket       *transport.WSClient
	mutex           sync.RWMutex
	//redis      *transport.RedisManager
}

type tradesManagerInterface interface {
	SetIsLeader(ctx context.Context, isLeader bool)
	StreamData(ctx context.Context)
	UpdateAssets(ctx context.Context, assets []string)
}

var _ tradesManagerInterface = (*TradesManager)(nil)

func NewTradesManager(appContext *types.ExchangeConfig, assetsManager *assets.AssetsService, settingsManager *dynamicsettings.DynamicsettingsManager) *TradesManager {
	service := &TradesManager{
		assets:          appContext.Assets,
		assetsManager:   assetsManager,
		settingsManager: settingsManager,
		config:          appContext.Config,
		isLeader:        false,
		logger:          appContext.Logger,
		maxSpreadPct:    appContext.Config.MaxSpreadPct,
		nats:            appContext.Nats,
		//redis:    appContext.Redis,
	}

	streamNames := []string{}
	//es.assets = []string{"BTCUSDT"} // dummy data

	for _, asset := range service.assets {
		streamName := fmt.Sprintf("%s@trade", strings.ToLower(asset))
		streamNames = append(streamNames, streamName)
	}

	combinedStreamsURL := service.config.Exchanges.Binance.StreamWSURL + "?streams=" + strings.Join(streamNames, "/")
	service.Websocket = transport.NewWSClient(combinedStreamsURL,
		time.Duration(service.config.WsPingPeriod),
		service.config.WsPinMaxError,
		service.config.MaxRetry,
		time.Duration(service.config.MaxRetry),
		service.logger)

	return service
}

func (es *TradesManager) SetIsLeader(ctx context.Context, isLeader bool) {
	es.isLeader = isLeader
}

func (es *TradesManager) WSInstance(ctx context.Context) *transport.WSClient {
	return es.Websocket
}

func (es *TradesManager) UpdateAssets(ctx context.Context, assets []string) {
	es.mutex.Lock()
	defer es.mutex.Unlock()
	es.assets = assets
}

func (es *TradesManager) SetMaxSpread(ctx context.Context, maxSpread float64) {
	es.mutex.Lock()
	defer es.mutex.Unlock()
	if maxSpread >= 0 {
		es.maxSpreadPct = maxSpread
	}
}

func (es *TradesManager) StreamData(ctx context.Context) {

	for {
		err := es.Websocket.Connect()
		if err != nil {
			es.logger.Warn("error connecting to binance ws:", zap.Error(err))
			time.Sleep(time.Second * time.Duration(es.config.MaxWait) * time.Duration(es.config.EtcdNodes))
			continue
		}

		go es.Websocket.MonitorConnection()

		for {
			var payload map[string]interface{}
			err := es.Websocket.Connection.ReadJSON(&payload)
			if err != nil {
				es.logger.Error("Ws connection read error", zap.Error(err))
				es.Websocket.Connection.Close()
				break
			}

			stream := payload["stream"].(string)
			data := payload["data"].(map[string]interface{})
			jsonData, err := json.Marshal(data)
			if err != nil {
				es.logger.Error("Error marshalling data", zap.Error(err))
			}

			streamData := TradesData{}

			if err := json.Unmarshal(jsonData, &streamData); err != nil {
				es.logger.Error("Failed to unmarshal data for "+streamData.Symbol, zap.String("symbol", streamData.Symbol), zap.Error(err))
			}

			es.streamData = &streamData
			es.streamData.Side = streamData.OrderSide()

			adjustedJsonData, err := json.Marshal(streamData)
			if err != nil {
				es.logger.Error("error marshalling adjusted data", zap.Error(err))
				continue
			}

			if es.isLeader {
				symbol := strings.ToUpper(strings.Split(stream, "@")[0])
				subject := fmt.Sprintf("trades.%s", symbol)
				if err := es.nats.Publish(subject, adjustedJsonData); err != nil {
					es.logger.Error("Error publishing to "+subject, zap.Error(err))
				}
			}

		}

		es.Websocket.Connection.Close()
	}
}

func (td *TradesData) OrderSide() string {
	if td.MarketMaker {
		return "SELL"
	}
	return "BUY"
}
