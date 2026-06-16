package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/scriptease/database-cli/internal/jsonerror"
)

type Client struct {
	http *http.Client
}

func New(socketPath string) *Client {
	transport := &http.Transport{
		DisableCompression: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	return &Client{
		http: &http.Client{Transport: transport},
	}
}

func (c *Client) Get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, "", nil)
}

func (c *Client) PostJSON(path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do(http.MethodPost, path, "application/json", bytes.NewReader(body))
}

func (c *Client) PostText(path string, contentType string, payload []byte) ([]byte, error) {
	return c.do(http.MethodPost, path, contentType, bytes.NewReader(payload))
}

func (c *Client) do(method string, path string, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, "http://unix"+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon request failed: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, jsonerror.NewRaw(payload)
	}
	return payload, nil
}
