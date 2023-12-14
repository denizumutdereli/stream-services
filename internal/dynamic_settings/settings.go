package dynamicsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"go.uber.org/zap"
)

type Settings struct {
	MaxSpreadPct float64
}

type DynamicsettingsManager struct {
	config         *config.Config
	settingsMux    sync.RWMutex
	logger         *zap.Logger
	redis          *transport.RedisManager
	PairSettings   map[string]Settings
	defaulSettings Settings
}

func NewDynamicsettingsManager(config *config.Config, logger *zap.Logger, redis *transport.RedisManager) (*DynamicsettingsManager, error) {
	var ctx = context.Background()
	service := &DynamicsettingsManager{
		config:         config,
		logger:         logger,
		redis:          redis,
		PairSettings:   make(map[string]Settings),
		defaulSettings: Settings{MaxSpreadPct: config.MaxSpreadPct},
	}

	var wg sync.WaitGroup

	// Increment the wait group counter for each goroutine
	wg.Add(1)

	go func() {
		defer wg.Done() // Decrement the counter when the goroutine completes
		service.RefreshSettings(ctx)
	}()

	// Wait for all goroutines to complete
	wg.Wait()

	go func() {
		service.startPeriodicUpdate(ctx)
	}()

	return service, nil
}

// func (d *DynamicsettingsManager) getInitialSettings(ctx context.Context) {
// 	restClient := transport.NewRestClient(d.config.SettingsApi, d.config.Logger)
// 	path := "/"
// 	redisKey := "settings"
// 	settings, err := d.UpdateSettings(ctx, d.redis, restClient, redisKey, path)
// 	if err != nil {
// 		log.Fatalf("Fatal error getting dynamicsettings: %v", err)
// 	}

// 	fmt.Println("init", settings)

// 	for k, v := range *settings {
// 		if k == "DEFAULT" {
// 			d.defaulSettings = Settings(v)
// 		} else {
// 			if v.MaxSpreadPct >= 0 || v.MaxSpreadPct > 100 {
// 				v.MaxSpreadPct = d.config.MaxSpreadPct
// 			}
// 			d.PairSettings[k] = Settings{MaxSpreadPct: v.MaxSpreadPct}
// 		}
// 	}
// }

func (d *DynamicsettingsManager) UpdateSettings(ctx context.Context, redisClient *transport.RedisManager, restClient *transport.Client, redisKey string, path string) (*map[string]Settings, error) {
	responseStruct := make(map[string]Settings)

	response, err := restClient.DoRequest("GET", path, nil, nil)
	if err != nil {
		d.logger.Error("Failed to do request to REST API:", zap.Error(err))
		return nil, err
	}

	err = json.Unmarshal(response.Body, &responseStruct)
	if err != nil {
		d.logger.Error("Failed to unmarshal response:", zap.Error(err))
		return nil, err
	}

	// Cache the new data
	err = redisClient.SetKeyValue(ctx, redisKey, responseStruct)
	if err != nil {
		d.logger.Error("Failed to set key-value in Redis:", zap.Error(err))
	}

	return &responseStruct, nil
}

func (d *DynamicsettingsManager) startPeriodicUpdate(parentCtx context.Context) {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.RefreshSettings(parentCtx)
		case <-parentCtx.Done():
			log.Println("Stopping periodic updates.")
			return
		}
	}
}

func (d *DynamicsettingsManager) RefreshSettings(ctx context.Context) {
	settings, err := d.UpdateSettings(ctx, d.redis, transport.NewRestClient(d.config.SettingsApi, d.config.Logger), "settings", "/")
	if err != nil {
		log.Printf("Error updating settings: %v", err)
		return
	}

	fmt.Println("refresh:", settings)
	d.settingsMux.Lock()
	defer d.settingsMux.Unlock()

	d.PairSettings = make(map[string]Settings)

	for k, v := range *settings {
		if k == "DEFAULT" {
			d.defaulSettings = Settings(v)
		} else {
			d.PairSettings[k] = Settings{MaxSpreadPct: v.MaxSpreadPct}
		}
	}
}

func (d *DynamicsettingsManager) GetSettingsForSymbol(symbol string) Settings {
	d.settingsMux.RLock()
	defer d.settingsMux.RUnlock()
	if settings, ok := d.PairSettings[symbol]; ok {
		return settings
	}

	return d.GetDefaultSettings()
}

func (d *DynamicsettingsManager) GetDefaultSettings() Settings {
	d.settingsMux.RLock()
	defer d.settingsMux.RUnlock()
	fmt.Println("---->", d.PairSettings["DEFAULT"])
	return d.PairSettings["DEFAULT"]
}
