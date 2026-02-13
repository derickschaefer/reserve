// Package fred implements the HTTP client for the Federal Reserve Bank of
// St. Louis (FRED) API. All methods are context-aware, respect the shared
// rate limiter, and retry on transient errors (429, 5xx).
//
// The package is split across multiple files, each covering one FRED resource:
//
//	client.go   — Client struct, NewClient, low-level get()
//	series.go   — series endpoints
//	category.go — category endpoints
//	release.go  — release endpoints
//	source.go   — source endpoints
//	tag.go      — tag endpoints
package fred

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL = "https://api.stlouisfed.org/fred/"
	maxRetries     = 4
)

// Client is the FRED API HTTP client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter
	debug      bool
}

// NewClient creates a Client with the given API key, base URL, timeout,
// rate limit (requests/sec), and debug flag.
func NewClient(apiKey, baseURL string, timeout time.Duration, ratePerSec float64, debug bool) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	burst := int(ratePerSec)
	if burst < 1 {
		burst = 1
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		limiter: rate.NewLimiter(rate.Limit(ratePerSec), burst),
		debug:   debug,
	}
}

// get performs a GET request to the FRED API, handling rate limiting and retries.
func (c *Client) get(ctx context.Context, endpoint string, params url.Values, out interface{}) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}

	params.Set("api_key", c.apiKey)
	params.Set("file_type", "json")

	reqURL := c.baseURL + endpoint + "?" + params.Encode()

	if c.debug {
		safe := strings.Replace(reqURL, c.apiKey, "REDACTED", 1)
		slog.Debug("fred request", "url", safe)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
			slog.Debug("retrying after backoff", "attempt", attempt, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "reserve-cli/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading body: %w", err)
			continue
		}

		if c.debug {
			slog.Debug("fred response", "status", resp.StatusCode, "bytes", len(body))
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var apiErr struct {
				Error string `json:"error_message"`
			}
			_ = json.Unmarshal(body, &apiErr)
			if apiErr.Error != "" {
				return fmt.Errorf("API error: %s", apiErr.Error)
			}
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}
