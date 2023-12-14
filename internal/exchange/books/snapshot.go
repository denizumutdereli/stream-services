package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/denizumutdereli/stream-services/internal/transport"
	"github.com/denizumutdereli/stream-services/internal/utils"
	"go.uber.org/zap"
)

func (es *OrderbookManager) fetchInitialOrderbook(ctx context.Context, symbol string) error {

	if utils.Contains(es.config.IgnoredAsssets, symbol) {
		return nil
	}

	baseUrl := es.config.Exchanges.Binance.RestURL
	restClient := transport.NewRestClient(baseUrl, es.config.Logger)
	symbolPath := fmt.Sprintf("depth?symbol=%s&limit=%d", symbol, es.config.Exchanges.Binance.SnapshotDepth)
	path := symbolPath

	var err error
	for attempt := 1; attempt <= es.config.MaxRetry; attempt++ {

		response, err := restClient.DoRequest("GET", path, nil, nil)
		if err == nil {
			var orderbook *Orderbook
			err = json.Unmarshal(response.Body, &orderbook)
			if err != nil {
				log.Printf("Failed to unmarshal response: %v\n", err)
				es.logger.Warn("Failed to unmarshal response:", zap.Error(err))

				return err
			}

			orderbook.Symbol = symbol

			es.orderbooks.Store(symbol, orderbook)
			//es.sortOrderbookInBackground(symbol)

			//es.logger.Debug("Fetched initial orderbook for " + symbol + " number " + fmt.Sprint(i))
			return nil
		}

		es.logger.Warn("Failed to fetch initial orderbook for ", zap.String("symbol", symbol),
			zap.Int("attempt", attempt), zap.Int("MaxRetry", es.config.MaxRetry), zap.Error(err))

		es.addAssetToRecovery(ctx, symbol)
		continue
		//time.Sleep(time.Second * time.Duration(es.config.MaxWait))
		//time.Sleep(time.Second * 5)
	}

	return err
}

func (es *OrderbookManager) addAssetToRecovery(ctx context.Context, symbol string) {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	if _, exists := es.recoveryAssets[symbol]; !exists {
		es.recoveryAssets[symbol] = true
	}
}
