package transport

import (
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type NatsManager struct {
	nc               *nats.Conn
	logger           *zap.Logger
	wg               sync.WaitGroup
	connectionStatus bool
	connectionMtx    sync.Mutex
}

type natsManagerInterface interface {
	Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error)
	Publish(subject string, data []byte) error
	// setupConnOptions(opts []nats.Option) []nats.Option
}

var _ natsManagerInterface = (*NatsManager)(nil)

/*
nm := NewNatsManager([]string{"nats://nats1:4222", "nats://nats2:4222", "nats://nats3:4222"})
*/

func NewNatsManager(servers []string, logger *zap.Logger) (*NatsManager, error) {
	opts := []nats.Option{nats.Name("NATS Manager")}
	opts = setupConnOptions(opts, logger)

	serversStr := strings.Join(servers, ",")

	nc, err := nats.Connect(serversStr, opts...)
	if err != nil {
		return nil, err
	}

	service := &NatsManager{nc: nc, logger: logger}
	go service.heartbeat("test.*")

	return service, nil
}

func (nm *NatsManager) IsConnected() bool {
	nm.connectionMtx.Lock()
	defer nm.connectionMtx.Unlock()
	return nm.connectionStatus
}

func (nm *NatsManager) heartbeat(subject string) {
	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ticker.C:
			heartbeatMsg := "heartbeat"
			respChan := make(chan *nats.Msg, 1)

			sub, err := nm.nc.Subscribe(subject, func(m *nats.Msg) {
				respChan <- m
			})
			if err != nil {
				nm.logger.Error("Error subscribing to response subject", zap.Error(err))
				nm.setConnectionStatus(false)
				continue
			}

			err = nm.nc.Publish(subject, []byte(heartbeatMsg))
			if err != nil {
				nm.logger.Error("Error sending heartbeat message", zap.Error(err))
				nm.setConnectionStatus(false)
				continue
			}

			select {
			case <-time.After(5 * time.Second):
				nm.logger.Info("Heartbeat timeout reached, no response")
				nm.setConnectionStatus(false)
			case <-respChan:
				nm.setConnectionStatus(true)
			}

			sub.Unsubscribe()
		}
	}
}

func (nm *NatsManager) setConnectionStatus(status bool) {
	nm.connectionMtx.Lock()
	defer nm.connectionMtx.Unlock()
	nm.connectionStatus = status
}

func (nm *NatsManager) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {

	sub, err := nm.nc.Subscribe(subject, func(m *nats.Msg) {
		nm.wg.Add(1)

		handler(m)
		nm.wg.Done()
	})

	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (nm *NatsManager) QueueSubscribe(subject, workergroup string, handler nats.MsgHandler) (*nats.Subscription, error) {

	sub, err := nm.nc.QueueSubscribe(subject, workergroup, func(m *nats.Msg) {
		nm.wg.Add(1)

		handler(m)
		nm.wg.Done()
	})

	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (nm *NatsManager) Publish(subject string, data []byte) error {
	return nm.nc.Publish(subject, data)
}

func (nm *NatsManager) Close() {
	// Gracefully close the connection
	nm.nc.Drain()

	// Wait for all subscriptions to finish
	nm.wg.Wait()

	nm.nc.Close()
}

func setupConnOptions(opts []nats.Option, logger *zap.Logger) []nats.Option {
	totalWait := 10 * time.Minute
	reconnectDelay := time.Second

	opts = append(opts, nats.ReconnectWait(reconnectDelay))
	opts = append(opts, nats.MaxReconnects(int(totalWait/reconnectDelay)))
	opts = append(opts, nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
		logger.Info("Got disconnected", zap.String("reason", err.Error()))
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		logger.Info("Got reconnected", zap.String("url", nc.ConnectedUrl()))
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		logger.Info("Connection closed", zap.String("reason", nc.LastError().Error()))
	}))
	return opts
}
