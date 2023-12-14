package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/denizumutdereli/stream-services/internal/transport"
	"go.uber.org/zap"
)

const (
	path      = "/"
	redis_key = "asset_pairs"
)

var err error

type AssetsService struct {
	assets        []string
	updatedAssets []string
	// missingAssets    []string
	// newAssets        []string
	ctx              context.Context
	cancel           context.CancelFunc
	config           *config.Config
	assetMap         map[string]bool
	logger           *zap.Logger
	redis            *transport.RedisManager
	restClient       *transport.Client
	connectionStatus bool
}

func NewAssetsService(config *config.Config, logger *zap.Logger, redis *transport.RedisManager) (*AssetsService, error) {
	service := &AssetsService{
		config:           config,
		logger:           logger,
		redis:            redis,
		assetMap:         make(map[string]bool),
		connectionStatus: true,
		restClient:       transport.NewRestClient(config.AssetsApi, logger),
	}

	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	service.cancel = cancel

	err := service.initAssets(ctx)
	if err != nil {
		service.logger.Fatal("Error updating assets", zap.Error(err))
	}

	// go func() {
	// 	service.periodicUpdate(ctx)
	// }()

	return service, nil
}

func (es *AssetsService) SetAssetsFromExternalService(ctx context.Context, assets []string) error {
	es.assets = assets
	return nil
}

func (es *AssetsService) GetAssets() ([]string, error) {

	if len(es.assets) == 0 {
		return nil, fmt.Errorf("initial assets are not available")
	}

	return es.assets, nil
}

func (es *AssetsService) initAssets(ctx context.Context) error {
	if es.assets, err = es.fetchAssets(ctx, false); err != nil {
		return err
	}

	return nil
}

// func (es *AssetsService) periodicUpdate(ctx context.Context) {
// 	ticker := time.NewTicker(10 * time.Second)
// 	defer ticker.Stop()

// 	for {

// 		select {
// 		case <-ctx.Done():
// 			continue // TODO: recover
// 		case <-ticker.C:

// 			es.updatedAssets, err = es.fetchAssets(ctx, true)

// 			fmt.Println("Updated assets: ", es.updatedAssets, "**************************************************")

// 			if err != nil {
// 				es.logger.Error("Failed to fetch assets", zap.Error(err))
// 				es.connectionStatus = false
// 			}

// 			if len(es.updatedAssets) == 0 {
// 				es.logger.Debug("empty updatedAssets")
// 			}
// 			es.connectionStatus = true
// 			//fmt.Println("Updating assets.....", es.updatedAssets)
// 		}

// 	}
// }

func (es *AssetsService) CompareAssets() (updatedAssets, newAssets, missingAssets []string) {

	updatedAssetSet := make(map[string]bool)

	for _, asset := range es.updatedAssets {
		updatedAssetSet[asset] = true
	}

	// Check for new assets
	for _, asset := range es.updatedAssets {
		if !es.assetMap[asset] {
			newAssets = append(newAssets, asset)
		}
	}

	// Check for missing assets
	for _, asset := range es.assets {
		if !updatedAssetSet[asset] {
			missingAssets = append(missingAssets, asset)
		}
	}

	fmt.Println("assets:", es.updatedAssets, newAssets, missingAssets)

	return es.updatedAssets, newAssets, missingAssets
}

func (es *AssetsService) fetchAssets(ctx context.Context, newData bool) ([]string, error) {
	var cachedData struct {
		Data []string `json:"data"`
	}

	if !newData {
		// fmt.Println("reading from redis")
		err := es.redis.GetKeyValue(ctx, redis_key, &cachedData)
		if err == nil {
			for i, v := range cachedData.Data {
				cachedData.Data[i] = strings.ToLower(strings.ReplaceAll(v, "-", ""))
			}
			return cachedData.Data, nil
		}

	}

	// fmt.Println("reading from api")
	// Cache does not exist, is malformed, or expired, fetch asset pairs from REST API
	response, err := es.restClient.DoRequest("GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var responseStruct struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}

	if err := json.Unmarshal(response.Body, &responseStruct); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract and normalize pairs
	var pairs []string
	for _, result := range responseStruct.Result {
		normalizedPairName := strings.ToLower(strings.ReplaceAll(result.Name, "-", ""))
		pairs = append(pairs, normalizedPairName)
	}

	err = es.redis.SetKeyValue(ctx, redis_key, map[string][]string{"data": pairs}, time.Hour*24) // Todo: from config
	if err != nil {
		return nil, err
	}

	return pairs, nil
}
