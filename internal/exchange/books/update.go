package exchange

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	dynamicsettings "github.com/denizumutdereli/stream-services/internal/dynamic_settings"
	"github.com/denizumutdereli/stream-services/internal/utils"
	"go.uber.org/zap"
)

func (es *OrderbookManager) getMidPrice(symbol string) float64 {
	value, ok := es.orderbooks.Load(symbol)

	if !ok {
		return 0
	}

	orderbook := value.(*Orderbook)

	bids := orderbook.Bids
	asks := orderbook.Asks

	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	bestBidPrice := utils.ParseFloat(bids[0][0])
	bestAskPrice := utils.ParseFloat(asks[0][0])

	return (bestBidPrice + bestAskPrice) / 2
}

func (es *OrderbookManager) updateOrder(slice *[][2]string, order [2]string, isBid bool, symbol string, symbolSettings dynamicsettings.Settings) {
	price, qty := order[0], order[1]
	priceFloat := utils.ParseFloat(price)

	qtyFloat, err := strconv.ParseFloat(qty, 64)
	if err != nil {
		es.logger.Error("Failed to parse qty", zap.Error(err))
		return
	}

	// Check for spread
	midPrice := es.getMidPrice(symbol)
	if midPrice != 0 && (priceFloat < midPrice*(1-symbolSettings.MaxSpreadPct/100) || priceFloat > midPrice*(1+symbolSettings.MaxSpreadPct/100)) {
		//es.logger.Debug("Order price spread too high. Ignoring update.", zap.Float64("Price", priceFloat), zap.Float64("Mid-price", midPrice), zap.Float64("Max-spread", symbolSettings.MaxSpreadPct))
		return
	}

	// If the quantity is 0, remove the order
	// if qtyFloat == 0 {
	// 	for i, existingOrder := range *slice {
	// 		if existingOrder[0] == price {
	// 			*slice = append((*slice)[:i], (*slice)[i+1:]...)
	// 			return
	// 		}
	// 	}
	// 	return
	// }

	// If the quantity is 0, remove the order
	if qtyFloat == 0 { // here Im checking double due to if there is an unknown random pair name
		for i, existingOrder := range *slice {
			if existingOrder[0] == price {
				if i == len(*slice)-1 {
					// If i is the last index, simply remove the last element
					*slice = (*slice)[:i]
				} else {
					// Remove the element at index i by combining slices before and after it
					*slice = append((*slice)[:i], (*slice)[i+1:]...)
				}
				return
			}
		}
		return
	}

	i := sort.Search(len(*slice), func(i int) bool {
		if isBid {
			return (*slice)[i][0] <= price
		}
		return (*slice)[i][0] >= price
	})

	// If the price matches an existing order, update its quantity
	if i < len(*slice) && (*slice)[i][0] == price {
		(*slice)[i][1] = qty
	} else {
		*slice = append(*slice, [2]string{})
		copy((*slice)[i+1:], (*slice)[i:])
		(*slice)[i] = order
	}
}

func (es *OrderbookManager) handleStreamUpdate(ctx context.Context, update StreamUpdate) {

	var missingOrderbooks []string
	value, ok := es.orderbooks.Load(update.Symbol)

	if !ok {
		es.logger.Info("Orderbook not initialized fetching now: ", zap.String("symbol", update.Symbol))

		missingOrderbooks = append(missingOrderbooks, update.Symbol)

		if len(missingOrderbooks) >= es.config.Exchanges.Binance.SnapshotChunkSize {
			es.getSnapshots(context.TODO(), missingOrderbooks)
		}

		return
	}

	orderbook := value.(*Orderbook)

	symbolSettings := es.symbolSettingsGether(update.Symbol)

	if orderbook.LastUpdateID != 0 && update.FirstUpdateID != orderbook.LastUpdateID+1 {
		// es.logger.Warn("Mismatch in update IDs for: "+update.Symbol+
		// 	" Expected: "+fmt.Sprint(orderbook.LastUpdateID+1)+" Got: "+fmt.Sprint(update.FirstUpdateID),
		// 	zap.String("symbol", update.Symbol), zap.Int("expected", int(orderbook.LastUpdateID+1)), zap.Int("got", int(update.FirstUpdateID)))

		es.mismatchCount[update.Symbol]++

		fmt.Println(es.mismatchCount[update.Symbol], update.Symbol)
		if es.mismatchCount[update.Symbol] >= 2 {
			es.fetchInitialOrderbook(context.Background(), update.Symbol)
			return
		}
	} else {
		es.mismatchCount[update.Symbol] = 0
	}

	for _, bid := range update.BidsToBeUpdated {
		es.updateOrder(&orderbook.Bids, bid, true, update.Symbol, symbolSettings)
	}

	for _, ask := range update.AsksToBeUpdated {
		es.updateOrder(&orderbook.Asks, ask, false, update.Symbol, symbolSettings)
	}

	orderbook.LastUpdateID = update.FinalUpdateID
	//es.orderbooks.Store(update.Symbol, orderbook)
	es.sortOrderbookInBackground(update.Symbol, symbolSettings)
}
