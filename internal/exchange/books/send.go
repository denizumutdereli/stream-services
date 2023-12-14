package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"go.uber.org/zap"
)

func (es *OrderbookManager) SendToNats(ctx context.Context) {
	es.processChannel(ctx, es.messageChan)
}

func (es *OrderbookManager) processChannel(ctx context.Context, ch <-chan *StreamUpdate) {
	for update := range ch {

		es.handleStreamUpdate(ctx, *update)

		//symbol := strings.ToUpper(strings.Split(update.Symbol, "@")[0])
		symbol := update.Symbol
		value, exist := es.orderbooks.Load(update.Symbol)
		if !exist {
			es.logger.Warn("No orderbook found for symbol:", zap.String("symbol", symbol))
			es.streamUpdatePool.Put(update)
			continue
		}

		adjustedOrderbook := value.(*Orderbook)

		trimmedOrderbook := *adjustedOrderbook

		if len(trimmedOrderbook.Bids) > 250 {
			trimmedOrderbook.Bids = trimmedOrderbook.Bids[:250]
		}
		if len(trimmedOrderbook.Asks) > 250 {
			trimmedOrderbook.Asks = trimmedOrderbook.Asks[:250]
		}

		// markupRatio := es.markup.GetMarkupForSymbol(ctx, symbol)
		// orderbookWithMarkup := es.applyMarkupToOrderbook(ctx, &trimmedOrderbook, markupRatio)

		adjustedJsonData, err := json.Marshal(trimmedOrderbook)
		if err != nil {
			es.logger.Warn("Error marshalling adjusted data:", zap.Error(err))

			es.streamUpdatePool.Put(update)
			continue
		}

		if es.isLeader {
			subject := fmt.Sprintf("orderbook.%s", symbol)
			if err := es.nats.Publish(subject, adjustedJsonData); err != nil {
				log.Printf("error publishing to %s: %v", subject, err)
			}
		}
		*update = StreamUpdate{}
		es.streamUpdatePool.Put(update)
	}
}
