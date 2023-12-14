package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"
)

// TODO: proxy ip pooling

type Client struct {
	BaseURL string
	logger  *zap.Logger
}

type HTTPResponse struct {
	Body       []byte
	StatusCode int
}

func NewRestClient(baseURL string, logger *zap.Logger) *Client {
	return &Client{BaseURL: baseURL, logger: logger}
}

func (c *Client) DoRequest(method, path string, body interface{}, headers map[string]string) (*HTTPResponse, error) {
	var reqBody io.Reader

	if method != "GET" && body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			c.logger.Error("Failed to marshal body", zap.Error(err))
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	fullURL := c.BaseURL + path
	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		c.logger.Error("Failed to create new request", zap.Error(err))
		return nil, fmt.Errorf("failed to create new request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	if method != "GET" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Error("Failed to do request", zap.Error(err))
		return nil, fmt.Errorf("failed to do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read response body", zap.Error(err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Logging the error response body for debugging purposes
		c.logger.Error("Error response body", zap.String("response", string(respBody)))
		return &HTTPResponse{Body: respBody, StatusCode: resp.StatusCode}, fmt.Errorf("received non-OK HTTP status: %s", resp.Status)
	}

	return &HTTPResponse{Body: respBody, StatusCode: resp.StatusCode}, nil
}

func (c *Client) SetBaseURL(baseURL string) {
	c.BaseURL = baseURL
}
