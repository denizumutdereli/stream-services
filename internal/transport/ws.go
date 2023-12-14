package transport

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type WSClient struct {
	URL              *url.URL
	Connection       *websocket.Conn
	PingPeriod       time.Duration
	MaxPingError     int
	MaxRetry         int
	RetryWait        time.Duration
	pingErrors       int
	connectionChan   chan *bool
	connectionStatus bool
	mutex            sync.Mutex
	logger           *zap.Logger
}

func NewWSClient(u string, pingPeriod time.Duration, maxPingError int, maxRetry int, retryWait time.Duration, logger *zap.Logger) *WSClient {
	wsurl, err := url.Parse(u)
	if err != nil {
		log.Fatalf("Failed to parse WebSocket server URL: %v", err)
	}

	return &WSClient{
		URL:              wsurl,
		PingPeriod:       pingPeriod,
		MaxPingError:     maxPingError,
		MaxRetry:         maxRetry,
		RetryWait:        retryWait,
		pingErrors:       0,
		connectionChan:   make(chan *bool, 1),
		connectionStatus: false,
		logger:           logger,
	}
}
func (w *WSClient) Connect() error {

	go func() {
		for status := range w.connectionChan {
			w.mutex.Lock()
			w.connectionStatus = *status
			w.mutex.Unlock()
		}
	}()

	c, _, err := websocket.DefaultDialer.Dial(w.URL.String(), nil)
	if err != nil {
		w.connectionChan <- &[]bool{false}[0]
		return err
	}

	w.mutex.Lock()
	w.Connection = c
	w.connectionChan <- &[]bool{true}[0]
	w.mutex.Unlock()
	w.logger.Info("Websocket connected successfully")

	// Setup pong handler
	w.Connection.SetPongHandler(func(string) error {
		w.mutex.Lock()
		defer w.mutex.Unlock()
		w.pingErrors = 0
		return nil
	})

	return nil
}

func (w *WSClient) Disconnect() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.Connection != nil {
		w.Connection.Close()
		w.Connection = nil
		w.connectionChan <- &[]bool{false}[0]
		w.logger.Info("Disconnected")
	}
}

func (w *WSClient) Send(messageType int, data []byte) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.Connection == nil {
		return fmt.Errorf("websocket connection not established")
	}
	return w.Connection.WriteMessage(messageType, data)
}

func (w *WSClient) MonitorConnection() {
	ticker := time.NewTicker(time.Duration(w.PingPeriod) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if !w.checkConnection() {
			w.pingErrors++
			if w.pingErrors >= w.MaxPingError {
				w.attemptReconnect()
			}
		}
	}
}

func (w *WSClient) checkConnection() bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.Connection == nil {
		return false
	}

	err := w.Connection.WriteMessage(websocket.PingMessage, []byte{})
	return err == nil
}

func (w *WSClient) attemptReconnect() {
	w.Disconnect()
	retry := 0
	backoff := w.RetryWait * time.Second
	for retry <= w.MaxRetry {
		w.logger.Info("Attempting reconnect to websocket", zap.Int("attempt", retry+1))
		err := w.Connect()
		if err == nil {
			return
		}

		if retry == w.MaxRetry {
			w.logger.Fatal("Max retries reached. Failed to reconnect to websocket.")
		}

		retry++
		w.logger.Info("Failed reconnect to websocket", zap.Int("attempt", retry), zap.Duration("waiting_time", backoff))
		time.Sleep(backoff)
		backoff *= 2
	}
}

func (w *WSClient) IsConnected() bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.connectionStatus
}
