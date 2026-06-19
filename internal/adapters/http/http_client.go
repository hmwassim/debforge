package http

import (
	"net/http"

	"github.com/hmwassim/debforge/internal/ports"
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{},
	}
}

func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

var _ ports.HTTPClient = (*HTTPClient)(nil)
