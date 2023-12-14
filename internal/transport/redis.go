package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type RedisManager struct {
	Client           *redis.Client
	config           *config.Config
	logger           *zap.Logger
	mux              sync.RWMutex
	connectionMtx    sync.Mutex
	connectionStatus bool
}

type redisManagerInterface interface {
	IsConnected() bool
	heartbeat(ctx context.Context) error
	MonitorConnection(ctx context.Context)
	SetKeyValue(ctx context.Context, key string, value interface{}, expiration ...time.Duration) error
	GetKeyValue(ctx context.Context, key string, value interface{}) error
	PushList(ctx context.Context, key string, value interface{}) error
	TrimList(ctx context.Context, key string, start, stop int64) error
	DeleteKey(ctx context.Context, key string) error
	GetList(ctx context.Context, key string, start, stop int64) ([]interface{}, error)
	GetAll(ctx context.Context, prefix string) (map[string]interface{}, error)
}

var _ redisManagerInterface = (*RedisManager)(nil)

func NewRedisManager(redisURL string, cnf *config.Config) (*RedisManager, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %v", err)
	}

	client := redis.NewClient(options)

	ctx := context.Background()

	_, err = client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	redisClient := &RedisManager{
		Client:           client,
		connectionStatus: true,
		config:           cnf,
		logger:           cnf.Logger,
	}

	go redisClient.MonitorConnection(ctx)

	return redisClient, nil
}

func (r *RedisManager) IsConnected() bool {
	r.connectionMtx.Lock()
	defer r.connectionMtx.Unlock()
	return r.connectionStatus
}

func (r *RedisManager) heartbeat(ctx context.Context) error {
	_, err := r.Client.Ping(ctx).Result()
	if err != nil {
		r.setConnectionStatus(false)
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	r.setConnectionStatus(true)
	return nil
}

func (r *RedisManager) MonitorConnection(ctx context.Context) {
	r.logger.Info("Entered Redis MonitorConnection", zap.Bool("connectionStatus", r.connectionStatus))

	ticker := time.NewTicker(time.Duration(r.config.DefaultTickerInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := r.heartbeat(ctx)
			if err != nil {
				r.logger.Error("Failed to ping Redis", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *RedisManager) setConnectionStatus(status bool) {
	r.connectionMtx.Lock()
	defer r.connectionMtx.Unlock()
	r.connectionStatus = status
}

func (r *RedisManager) SetKeyValue(ctx context.Context, key string, value interface{}, expiration ...time.Duration) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	expire := time.Duration(0) // Default no expiration
	if len(expiration) > 0 {
		expire = expiration[0]
	}

	err = r.Client.Set(ctx, key, jsonValue, expire).Err()
	if err != nil {
		r.logger.Error("Error setting key in Redis", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func (r *RedisManager) GetKeyValue(ctx context.Context, key string, value interface{}) error {
	r.mux.RLock()
	defer r.mux.RUnlock()

	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(val), value)
	if err != nil {
		return err
	}

	return nil
}

func (r *RedisManager) PushList(ctx context.Context, key string, value interface{}) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = r.Client.LPush(ctx, key, jsonValue).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *RedisManager) TrimList(ctx context.Context, key string, start, stop int64) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	err := r.Client.LTrim(ctx, key, start, stop).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *RedisManager) DeleteKey(ctx context.Context, key string) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	err := r.Client.Del(ctx, key).Err()
	if err != nil {
		r.logger.Error("Error deleting key in Redis", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func (r *RedisManager) GetList(ctx context.Context, key string, start, stop int64) ([]interface{}, error) {
	vals, err := r.Client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}

	var results []interface{}
	for _, val := range vals {
		var data interface{}
		err = json.Unmarshal([]byte(val), &data)
		if err != nil {
			return nil, err
		}

		results = append(results, data)
	}

	return results, nil
}

func (r *RedisManager) GetAll(ctx context.Context, prefix string) (map[string]interface{}, error) {
	keys, err := r.Client.Keys(ctx, prefix+"*").Result()
	if err != nil {
		return nil, err
	}

	results := make(map[string]interface{})
	for _, key := range keys {
		val, err := r.Client.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}

		var data interface{}
		err = json.Unmarshal([]byte(val), &data)
		if err != nil {
			return nil, err
		}

		results[key] = data
	}

	return results, nil
}
