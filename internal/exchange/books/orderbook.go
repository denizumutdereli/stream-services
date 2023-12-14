package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

const (
	BidLevel = 3
	AskLevel = 3
)

type BidAndAsk []string

type Orderbook struct {
	Symbol       string      `json:"symbol"`
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"`
	Asks         [][2]string `json:"asks"`
}

type SortingTask struct {
	symbol string
	done   chan struct{}
}

type StreamUpdate struct {
	AsksToBeUpdated [][2]string `json:"a"`
	BidsToBeUpdated [][2]string `json:"b"`
	EventTime       int64       `json:"E"`
	EventType       string      `json:"e"`
	FinalUpdateID   int64       `json:"u"`
	FirstUpdateID   int64       `json:"U"`
	Symbol          string      `json:"s"`
}

type SendSubscriptionType struct {
	subs   bool
	assets []string
}

type OrderbookManager struct {
	assets []string
	// assetSettings        sync.Map
	defaultSettings      *dynamicsettings.Settings
	assetsManager        *assets.AssetsService
	settingsManager      *dynamicsettings.DynamicsettingsManager
	updatedAssets        []string
	missingAssets        []string
	newAssets            []string
	Websocket            *transport.WSClient
	nats                 *transport.NatsManager
	isLeader             bool
	config               *config.Config
	logger               *zap.Logger
	mutex                sync.RWMutex
	maxSpreadPct         float64
	sortingUpdatePool    *sync.Pool
	streamUpdatePool     *sync.Pool
	ctx                  context.Context
	cancel               context.CancelFunc
	messageChan          chan *StreamUpdate
	sortingChan          chan *SortingTask
	sendSubscriptionChan chan SendSubscriptionType
	orderbooks           sync.Map
	mismatchCount        map[string]int
	recoveryAssets       map[string]bool
	assetMap             map[string]bool
}

type orderbookManagerInterface interface {
	SetIsLeader(ctx context.Context, isLeader bool)
	WSInstance(ctx context.Context) *transport.WSClient
	// updateAssets(ctx context.Context)
	IsSnapshotExist(symbol string) bool
	snapshotInChunks(ctx context.Context)
	sendOrder(ctx context.Context) error
	getSnapshots(ctx context.Context, assets []string)
	recoverAssets(ctx context.Context) error
	WSConnection(ctx context.Context) error
	handleDisconnect(ctx context.Context)
	WsSendMessage(ctx context.Context, Request map[string]interface{}) error
	listenToWebSocket(ctx context.Context, channel chan *StreamUpdate) error
	getStreamNamesForSubscription(ctx context.Context, assets []string) ([]string, error)
	subscribeToStreams(ctx context.Context, subsOrUnSubs bool, streamNames []string) map[string]interface{}
	SendToNats(ctx context.Context)
	processChannel(ctx context.Context, ch <-chan *StreamUpdate)
	fetchInitialOrderbook(ctx context.Context, symbol string) error
	addAssetToRecovery(ctx context.Context, symbol string)
	sortOrderbookInBackground(symbol string, symbolSettings dynamicsettings.Settings)
	sortAndFixSpread(symbol string, symbolSettings dynamicsettings.Settings)
	getMidPrice(symbol string) float64
	updateOrder(slice *[][2]string, order [2]string, isBid bool, symbol string, symbolSettings dynamicsettings.Settings)
	handleStreamUpdate(ctx context.Context, update StreamUpdate)
	loadDefaultSettings(ctx context.Context)
	symbolSettingsGether(symbol string) dynamicsettings.Settings
	//startPeriodicUpdateSettings(parentCtx context.Context)
}

var _ orderbookManagerInterface = (*OrderbookManager)(nil)

func NewOrderbookManager(appContext *types.ExchangeConfig, assetsManager *assets.AssetsService, settingsManager *dynamicsettings.DynamicsettingsManager) *OrderbookManager {

	service := &OrderbookManager{
		assets:          appContext.Assets,
		assetsManager:   assetsManager,
		settingsManager: settingsManager,
		updatedAssets:   []string{},
		missingAssets:   []string{},
		newAssets:       []string{},
		assetMap:        make(map[string]bool),
		config:          appContext.Config,
		logger:          appContext.Logger,
		isLeader:        false,
		streamUpdatePool: &sync.Pool{
			New: func() interface{} {
				return &StreamUpdate{}
			},
		},
		sortingUpdatePool: &sync.Pool{
			New: func() interface{} {
				return &SortingTask{}
			},
		},
		messageChan:          make(chan *StreamUpdate, 3000),
		sortingChan:          make(chan *SortingTask, 3000),
		maxSpreadPct:         appContext.Config.MaxSpreadPct,
		mismatchCount:        make(map[string]int),
		nats:                 appContext.Nats,
		recoveryAssets:       make(map[string]bool),
		sendSubscriptionChan: make(chan SendSubscriptionType, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	service.cancel = cancel

	service.loadDefaultSettings(ctx)

	service.Websocket = transport.NewWSClient(service.config.Exchanges.Binance.StreamWSURL,
		time.Duration(service.config.WsPingPeriod),
		service.config.WsPinMaxError,
		service.config.MaxRetry,
		time.Duration(service.config.MaxRetry),
		service.logger)

	go service.SendToNats(ctx)
	go service.sendOrder(ctx)

	go service.recoverAssets(ctx)

	//go service.startPeriodicUpdateSettings(ctx)
	return service
}

func (es *OrderbookManager) SetIsLeader(ctx context.Context, isLeader bool) {
	es.isLeader = isLeader
}

func (es *OrderbookManager) WSInstance(ctx context.Context) *transport.WSClient {
	return es.Websocket
}

func (es *OrderbookManager) loadDefaultSettings(ctx context.Context) {

	defaultSettings := es.symbolSettingsGether("DEFAULT")

	fmt.Println(defaultSettings, "****************************************")

	if defaultSettings.MaxSpreadPct >= 0 || defaultSettings.MaxSpreadPct > 50 {
		defaultSettings.MaxSpreadPct = es.config.MaxSpreadPct
	}
	es.maxSpreadPct = defaultSettings.MaxSpreadPct

	es.defaultSettings = &defaultSettings

}

func (es *OrderbookManager) symbolSettingsGether(symbol string) dynamicsettings.Settings {
	return es.settingsManager.GetSettingsForSymbol(symbol)
}

// func (es *OrderbookManager) updateAssets(ctx context.Context) {
// 	es.mutex.Lock()
// 	defer es.mutex.Unlock()

// 	ticker := time.NewTicker(10 * time.Second) // todo: from config
// 	defer ticker.Stop()

// 	for {

// 		select {
// 		case <-ctx.Done():
// 			continue // TODO: recover

// 		case <-ticker.C:
// 			updatedAssets, newAssets, missingAssets := es.assetsManager.CompareAssets()

// 			fmt.Println("Orderbook Update Assets..............................................")
// 			fmt.Println(
// 				"initialAssets:", es.assets,
// 				"updateAssets:", updatedAssets,
// 				"newAssets:", newAssets,
// 				"missingAssets:", missingAssets,
// 				"----------------------------------------------------------------------------------")

// 			// if len(updatedAssets) > 0 {

// 			// 	if len(missingAssets) > 0 {
// 			// 		es.sendSubscriptionChan <- SendSubscriptionType{false, missingAssets}
// 			// 	}

// 			// 	// send new assets to recovery
// 			// 	if len(newAssets) > 0 {
// 			// 		es.partialInChunks(ctx, newAssets)
// 			// 	}

// 			// 	if len(es.newAssets) == 0 && len(es.missingAssets) == 0 {
// 			// 		es.mutex.Lock()
// 			// 		es.assets = updatedAssets
// 			// 		es.mutex.Unlock()
// 			// 	} else {
// 			// 		continue
// 			// 	}

// 			// }

// 			es.assetsManager.SetAssetsFromExternalService(ctx, es.assets)

// 		}
// 	}

// }

func (es *OrderbookManager) IsSnapshotExist(symbol string) bool {
	if _, ok := es.orderbooks.Load(symbol); ok {
		return true
	}

	return false
}

func (es *OrderbookManager) partialInChunks(ctx context.Context, assets []string) {
	chunkSize := es.config.Exchanges.Binance.SnapshotChunkSize

	for i := 0; i < len(es.assets); i += chunkSize {
		end := i + chunkSize
		if end > len(es.assets) {
			end = len(es.assets)
		}

		es.getSnapshots(ctx, es.assets[i:end])

		es.sendSubscriptionChan <- SendSubscriptionType{true, es.assets[i:end]}

	}
}

func (es *OrderbookManager) snapshotInChunks(ctx context.Context) {
	chunkSize := es.config.Exchanges.Binance.SnapshotChunkSize

	for i := 0; i < len(es.assets); i += chunkSize {
		end := i + chunkSize
		if end > len(es.assets) {
			end = len(es.assets)
		}

		es.getSnapshots(ctx, es.assets[i:end])

		es.sendSubscriptionChan <- SendSubscriptionType{true, es.assets[i:end]}

	}
}

func (es *OrderbookManager) sendOrder(ctx context.Context) error {

	for ch := range es.sendSubscriptionChan {

		select {
		case <-ctx.Done():
			return nil
		default:

			symbols := ch.assets
			sub := ch.subs

			es.logger.Debug("Subscription in place ------------------------->")
			streamNames, err := es.getStreamNamesForSubscription(ctx, symbols)
			fmt.Println("Orderbook Subs:", sub, streamNames)
			if err != nil {
				es.logger.Fatal("Error on building subs request.", zap.Error(err))
			}

			if !sub {
				request := es.subscribeToStreams(ctx, false, streamNames)
				suberr := es.WsSendMessage(ctx, request)

				if suberr != nil {
					es.logger.Fatal("Error on unsubscription request", zap.Error(err))
				}

			} else {

				request := es.subscribeToStreams(ctx, false, streamNames)
				suberr := es.WsSendMessage(ctx, request)

				if suberr != nil {
					es.logger.Fatal("Error on unsubscription request", zap.Error(err))
				}

				request = es.subscribeToStreams(ctx, true, streamNames)
				suberr = es.WsSendMessage(ctx, request)

				if suberr != nil {
					es.logger.Fatal("Error on subscription request", zap.Error(err))
				}

			}

		}

	}

	return nil
}

func (es *OrderbookManager) getSnapshots(ctx context.Context, assets []string) {
	weightPerMinute := 100
	interval := time.Minute / time.Duration(es.config.Exchanges.Binance.RateLimit/weightPerMinute)

	for _, asset := range assets {
		if !es.IsSnapshotExist(asset) {
			if err := es.fetchInitialOrderbook(ctx, strings.ToUpper(asset)); err != nil {
				es.logger.Error("Error fetching initial orderbook for", zap.String("asset:", asset), zap.Error(err))
			}
		}

		sleepDuration := interval
		if !es.isLeader {
			if es.config.EtcdNodes >= 5 {
				es.config.EtcdNodes = 5
			}
			sleepDuration *= time.Duration(es.config.EtcdNodes)
		}

		time.Sleep(sleepDuration)
	}
}

func (es *OrderbookManager) recoverAssets(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			es.mutex.Lock()

			recoverySymbols := make([]string, 0, len(es.recoveryAssets))
			for symbol := range es.recoveryAssets {
				recoverySymbols = append(recoverySymbols, symbol)
			}

			if len(recoverySymbols) > 0 {
				fmt.Println("Recover in place for symbols ----------------> ", recoverySymbols)
				es.partialInChunks(ctx, recoverySymbols)
				for symbol := range es.recoveryAssets {
					delete(es.recoveryAssets, symbol)
				}

			}
		case <-ctx.Done():
			return ctx.Err()
		}
		es.mutex.Unlock()
	}
}

func (es *OrderbookManager) WSConnection(ctx context.Context) error {
	err := es.Websocket.Connect()
	if err != nil {
		es.handleDisconnect(ctx)
		return err
	}

	go es.Websocket.MonitorConnection()

	return nil
}

func (es *OrderbookManager) handleDisconnect(ctx context.Context) {
	// es.reconnectMtx.Lock()
	// defer es.reconnectMtx.Unlock()
	// Cancel the context to stop the goroutines
	es.cancel()

	// Re-establish the WebSocket connection and restart the goroutines
	for {
		ctx, cancel := context.WithCancel(context.Background())
		es.ctx = ctx
		es.cancel = cancel

		err := es.Websocket.Connect()
		if err == nil {
			go es.StreamData(ctx)
			break
		}

		log.Printf("Error reconnecting: %v. Retrying in 5 seconds...", err)
		time.Sleep(5 * time.Second)
	}
}

func (es *OrderbookManager) WsSendMessage(ctx context.Context, Request map[string]interface{}) error {
	es.mutex.Lock()
	if err := es.Websocket.Connection.WriteJSON(Request); err != nil {
		es.logger.Error("Error sending request", zap.Error(err))
		//es.handleDisconnect(ctx)
		return err
	}
	es.mutex.Unlock()

	return nil
}

func (es *OrderbookManager) StreamData(ctx context.Context) {

	go es.snapshotInChunks(ctx)
	go es.listenToWebSocket(ctx, es.messageChan)

	// go func() {
	// 	// all done, now switching to assets synchronization
	// 	es.updateAssets(ctx)
	// }()

}

func (es *OrderbookManager) listenToWebSocket(ctx context.Context, channel chan *StreamUpdate) error {
	es.WSConnection(ctx)
	for {

		select {
		case <-es.ctx.Done():
			return nil // TODO: recover
		default:
			var payload map[string]interface{}
			err := es.Websocket.Connection.ReadJSON(&payload)
			if err != nil {
				es.Websocket.Connection.Close()
				return err
			}

			if payload["data"] == nil {
				continue
			}

			data, ok := payload["data"].(map[string]interface{})
			if !ok {
				es.logger.Warn("Error parsing data field")
				continue
			}

			jsonData, err := json.Marshal(data)
			if err != nil {
				es.logger.Warn("Error marshalling data", zap.Error(err))

				continue
			}

			streamData := es.streamUpdatePool.Get().(*StreamUpdate)

			if err := json.Unmarshal(jsonData, streamData); err != nil {
				es.streamUpdatePool.Put(streamData)
				continue
			}

			// Send the StreamUpdate object for processing
			channel <- streamData

		}

	}
}

func (es *OrderbookManager) getStreamNamesForSubscription(ctx context.Context, assets []string) ([]string, error) {
	var streamNames []string

	for _, asset := range assets {
		streamName := fmt.Sprintf("%s@depth@"+es.config.Exchanges.Binance.DefaultStreamInterval, strings.ToLower(asset))
		streamNames = append(streamNames, streamName)
	}

	return streamNames, nil

}

func (es *OrderbookManager) subscribeToStreams(ctx context.Context, subsOrUnSubs bool, streamNames []string) map[string]interface{} {

	var subscription string

	if subsOrUnSubs {
		subscription = "SUBSCRIBE"
	} else {
		subscription = "UNSUBSCRIBE"
	}

	subscriptionRequest := map[string]interface{}{
		"method": subscription,
		"params": streamNames,
		"id":     1,
	}

	return subscriptionRequest

}
