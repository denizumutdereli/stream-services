package exchange

/*
{
  "e": "24hrTicker",  // Event type
  "E": 123456789,     // Event time
  "s": "BNBBTC",      // Symbol
  "p": "0.0015",      // Price change
  "P": "250.00",      // Price change percent
  "w": "0.0018",      // Weighted average price
  "x": "0.0009",      // First trade(F)-1 price (first trade before the 24hr rolling window)
  "c": "0.0025",      // Last price
  "Q": "10",          // Last quantity
  "b": "0.0024",      // Best bid price
  "B": "10",          // Best bid quantity
  "a": "0.0026",      // Best ask price
  "A": "100",         // Best ask quantity
  "o": "0.0010",      // Open price
  "h": "0.0025",      // High price
  "l": "0.0010",      // Low price
  "v": "10000",       // Total traded base asset volume
  "q": "18",          // Total traded quote asset volume
  "O": 0,             // Statistics open time
  "C": 86400000,      // Statistics close time
  "F": 0,             // First trade ID
  "L": 18150,         // Last trade Id
  "n": 18151          // Total number of trades
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

type MarketData struct {
	EventType          string `json:"e"` // Event type
	EventTime          int64  `json:"E"` // Event time
	Symbol             string `json:"s"` // Symbol
	PriceChange        string `json:"p"` // Price change
	PriceChangePercent string `json:"P"` // Price change percent
	WeightedAverage    string `json:"w"` // Weighted average price
	// X                          string `json:"x"` // First trade(F)-1 price (first trade before the 24hr rolling window)
	LastPrice                  string `json:"c"` // Last price
	LastQuantity               string `json:"Q"` // Last quantity
	BestBidPrice               string `json:"b"` // Best bid price
	BestBidQuantity            string `json:"B"` // Best bid quantity
	BestAskPrice               string `json:"a"` // Best ask price
	BestAskQuantity            string `json:"A"` // Best ask quantity
	Open                       string `json:"o"` // Open price
	High                       string `json:"h"` // High price
	Low                        string `json:"l"` // Low price
	TotalTradeBaseAssetVolume  string `json:"v"` // Total traded base asset volume
	TotalTradeQuoteAssetVolume string `json:"q"` // Total traded quote asset volume
	StatisticOpenTime          int64  `json:"O"` // Statistics open time
	StatisticCloseTime         int64  `json:"C"` // Statistics close time
	FirstTradeId               int64  `json:"F"` // First trade ID
	LastTradeId                int64  `json:"L"` // Last trade Id
	TotalTrades                int64  `json:"n"` // Total number of trades
}

type MarketsManager struct {
	assets          []string
	assetsManager   *assets.AssetsService
	settingsManager *dynamicsettings.DynamicsettingsManager
	config          *config.Config
	incremental     map[string]MarketData
	isLeader        bool
	logger          *zap.Logger
	maxSpreadPct    float64
	mutex           sync.RWMutex
	nats            *transport.NatsManager
	redis           *transport.RedisManager
	snapshot        map[string]MarketData
	streamData      *MarketData
	Websocket       *transport.WSClient
}

type marketsManagerInterface interface {
	StreamData(ctx context.Context)
	SetIsLeader(ctx context.Context, isLeader bool)
	SetMaxSpread(ctx context.Context, maxSpread float64)
	SendToNats(ctx context.Context) error
	UpdateAssets(ctx context.Context, assets []string)
}

var _ marketsManagerInterface = (*MarketsManager)(nil)

func NewMarketsManager(appContext *types.ExchangeConfig, assetsManager *assets.AssetsService, settingsManager *dynamicsettings.DynamicsettingsManager) *MarketsManager {
	var ctx = context.Background()
	service := &MarketsManager{
		assets:          appContext.Assets,
		assetsManager:   assetsManager,
		settingsManager: settingsManager,
		config:          appContext.Config,
		incremental:     make(map[string]MarketData),
		logger:          appContext.Logger,
		maxSpreadPct:    appContext.Config.MaxSpreadPct,
		nats:            appContext.Nats,
		redis:           appContext.Redis,
		snapshot:        make(map[string]MarketData),
	}

	streamNames := []string{}
	//es.assets = []string{"BTCTRY", "BTCUSDT"}

	for _, asset := range service.assets {
		streamName := fmt.Sprintf("%s@ticker", strings.ToLower(asset))
		streamNames = append(streamNames, streamName)
	}

	combinedStreamsURL := service.config.Exchanges.Binance.StreamWSURL + "?streams=" + strings.Join(streamNames, "/")
	service.Websocket = transport.NewWSClient(combinedStreamsURL,
		time.Duration(service.config.WsPingPeriod),
		service.config.WsPinMaxError,
		service.config.MaxRetry,
		time.Duration(service.config.MaxRetry),
		service.logger)

	go service.SendToNats(ctx)

	return service
}

func (es *MarketsManager) SetIsLeader(ctx context.Context, isLeader bool) {
	es.isLeader = isLeader
}

func (es *MarketsManager) WSInstance(ctx context.Context) *transport.WSClient {
	return es.Websocket
}

func (es *MarketsManager) UpdateAssets(ctx context.Context, assets []string) {
	es.mutex.Lock()
	defer es.mutex.Unlock()
	es.assets = assets
}

func (es *MarketsManager) SetMaxSpread(ctx context.Context, maxSpread float64) {
	es.mutex.Lock()
	defer es.mutex.Unlock()
	if maxSpread >= 0 {
		es.maxSpreadPct = maxSpread
	}
}

func (es *MarketsManager) StreamData(ctx context.Context) {

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
			}

			data := payload["data"].(map[string]interface{})
			jsonData, err := json.Marshal(data)
			if err != nil {
				es.logger.Error("Error marshalling data", zap.Error(err))
			}

			streamData := MarketData{}

			if err := json.Unmarshal(jsonData, &streamData); err != nil {
				es.logger.Error("Failed to unmarshal data for "+streamData.Symbol, zap.String("symbol", streamData.Symbol), zap.Error(err))
			}

			es.streamData = &streamData

			if oldData, exists := es.snapshot[streamData.Symbol]; exists {
				if oldData.LastPrice != streamData.LastPrice {
					es.mutex.Lock()
					es.snapshot[streamData.Symbol] = streamData
					es.incremental[streamData.Symbol] = streamData
					es.mutex.Unlock()
					continue
				}

			} else {
				es.mutex.Lock()
				es.snapshot[streamData.Symbol] = streamData
				es.incremental[streamData.Symbol] = streamData
				es.mutex.Unlock()
			}

		}

	}
}

func (es *MarketsManager) SendToNats(ctx context.Context) error {

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {

		if !es.isLeader {
			continue
		}

		select {
		case <-ticker.C:
			es.mutex.Lock()
			if len(es.incremental) > 0 {
				adjustedJsonData, err := json.Marshal(es.incremental)
				if err != nil {
					es.logger.Error("error marshalling adjusted data", zap.Error(err))
				}

				adjustedJsonDataSnapshot, err := json.Marshal(es.snapshot)
				if err != nil {
					es.logger.Error("error marshalling adjusted snapshotdata", zap.Error(err))
					continue
				}

				subject := "markets.DATA"
				if err := es.nats.Publish(subject, adjustedJsonData); err != nil {
					es.logger.Error("Error publishing to "+subject, zap.Error(err))
				}

				subjectSnapshot := "markets.SNAPSHOT"
				if err := es.nats.Publish(subjectSnapshot, adjustedJsonDataSnapshot); err != nil {
					es.logger.Error("Error publishing to "+subjectSnapshot, zap.Error(err))
				}

				es.incremental = make(map[string]MarketData)
			}
			es.mutex.Unlock()
		}
	}

}
