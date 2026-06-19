package http

import (
	"net/http"
	"time"

	"github.com/hmwassim/debforge/internal/ports"
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

var _ ports.HTTPClient = (*HTTPClient)(nil)
