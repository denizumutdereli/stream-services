package exchange

import (
	"fmt"
	"sort"
	"sync"

	dynamicsettings "github.com/denizumutdereli/stream-services/internal/dynamic_settings"
	"github.com/denizumutdereli/stream-services/internal/utils"
	"go.uber.org/zap"
)

// func (es *OrderbookManager) startBackgroundSorting(ctx context.Context) {

// 	for task := range es.sortingChan {
// 		es.sortAndFixSpread(task.symbol)
// 		close(task.done)
// 		es.sortingUpdatePool.Put(task)
// 	}

// }

func (es *OrderbookManager) sortOrderbookInBackground(symbol string, symbolSettings dynamicsettings.Settings) {
	task := es.sortingUpdatePool.Get().(*SortingTask)
	task.symbol = symbol
	task.done = make(chan struct{})

	select {
	case es.sortingChan <- task:
	default:
		// The channel is full, execute the task synchronously
		es.sortAndFixSpread(symbol, symbolSettings)
		close(task.done)
	}
}

func (es *OrderbookManager) sortAndFixSpread(symbol string, symbolSettings dynamicsettings.Settings) {
	value, exists := es.orderbooks.Load(symbol)
	if !exists {
		fmt.Printf("Orderbook for %s not found\n", symbol)
		es.logger.Warn("Orderbook not found", zap.String("symbol", symbol))
		return
	}

	orderbook := value.(*Orderbook)

	bids := orderbook.Bids
	asks := orderbook.Asks

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sort.SliceStable(bids, func(i, j int) bool {
			return utils.ParseFloat(bids[i][0]) > utils.ParseFloat(bids[j][0])
		})
	}()

	go func() {
		defer wg.Done()
		sort.SliceStable(asks, func(i, j int) bool {
			return utils.ParseFloat(asks[i][0]) < utils.ParseFloat(asks[j][0])
		})
	}()

	wg.Wait()

	if len(bids) > 0 && len(asks) > 0 {
		bidPrice := utils.ParseFloat(bids[0][0])
		askPrice := utils.ParseFloat(asks[0][0])
		spreadPct := (askPrice - bidPrice) / bidPrice * 100

		if spreadPct > symbolSettings.MaxSpreadPct {
			midPrice := (bidPrice + askPrice) / 2
			allowedSpread := midPrice * (symbolSettings.MaxSpreadPct / 100)

			i := sort.Search(len(bids), func(i int) bool {
				return utils.ParseFloat(bids[i][0]) < midPrice-allowedSpread
			})
			bids = bids[:i]

			j := sort.Search(len(asks), func(i int) bool {
				return utils.ParseFloat(asks[i][0]) > midPrice+allowedSpread
			})
			asks = asks[:j]
		}
	}

	orderbook.Bids = bids
	orderbook.Asks = asks
}
