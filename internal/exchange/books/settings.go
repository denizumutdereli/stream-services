package exchange

// func (es *OrderbookManager) startPeriodicUpdateSettings(parentCtx context.Context) {
// 	ticker := time.NewTicker(time.Second * 30)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ticker.C:
// 			for _, symbol := range es.assets {
// 				settings := es.settingsManager.GetSettingsForSymbol(symbol)
// 				es.assetSettings.Store(symbol, settings)
// 			}

// 		case <-parentCtx.Done():
// 			log.Println("Stopping periodic updates.")
// 			return
// 		}
// 	}
// }
