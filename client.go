package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// HTTPClient executes JSON requests with bounded retries.
// A request succeeds only when the server returns a 2xx response and valid JSON;
// transport, HTTP status, and decoding errors are returned after all attempts.
type HTTPClient struct {
	Client  *http.Client
	Retries int
	Delay   time.Duration
	Logger  *log.Logger
}

// NewHTTPClient constructs a client with the same defaults as the Python version.
// Retries is the total number of attempts, not the number of additional retries.
func NewHTTPClient(timeout time.Duration, retries int, delay time.Duration, logger *log.Logger) (*HTTPClient, error) {
	if retries < 1 {
		return nil, fmt.Errorf("retries must be at least 1")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	if delay < 0 {
		return nil, fmt.Errorf("delay cannot be negative")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &HTTPClient{
		Client:  &http.Client{Timeout: timeout},
		Retries: retries,
		Delay:   delay,
		Logger:  logger,
	}, nil
}

// GetJSON performs a GET and decodes its response into out.
// The caller owns out; on failure it may contain partial decoder state and must
// not be used. Context cancellation stops further attempts and waiting.
func (c *HTTPClient) GetJSON(ctx context.Context, url string, headers http.Header, out any) error {
	var lastErr error
	for attempt := 1; attempt <= c.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header = headers.Clone()
		response, err := c.Client.Do(req)
		if err == nil {
			err = decodeResponse(response, out)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		c.Logger.Printf("GET %s attempt %d/%d failed: %v", url, attempt, c.Retries, err)
		if attempt < c.Retries {
			timer := time.NewTimer(c.Delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("GET %s failed after %d attempts: %w", url, c.Retries, lastErr)
}

func decodeResponse(response *http.Response, out any) error {
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	return nil
}
