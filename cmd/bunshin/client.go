package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// serverClient is a minimal HTTP client for talking to a bunshin serve instance.
type serverClient struct {
	baseURL string
	http    *http.Client
}

func newServerClient(addr string) *serverClient {
	return &serverClient{
		baseURL: addr,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// postJSON sends a JSON body to path and returns the decoded response.
func (c *serverClient) postJSON(path string, body, out any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(data)
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", r)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %s: %s", path, resp.Status, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// getJSON sends a GET to path and decodes the response into out.
func (c *serverClient) getJSON(path string, out any) error {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// putJSON sends a JSON body to path via PUT and optionally decodes the response.
func (c *serverClient) putJSON(path string, body, out any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPut, c.baseURL+path, r)
	if err != nil {
		return fmt.Errorf("PUT %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s: %s: %s", path, resp.Status, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// deleteHTTP sends a DELETE request to path. Returns an error for 4xx/5xx.
func (c *serverClient) deleteHTTP(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("DELETE %s: build request: %w", path, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: %s: %s", path, resp.Status, b)
	}
	return nil
}
