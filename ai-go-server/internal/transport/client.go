// Package transport contains the reusable HTTP client used for LLM requests.
package transport

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"
)

// HTTPConfig holds the connection-pool settings used for LLM HTTP requests.
type HTTPConfig struct {
	DialTimeout         time.Duration
	KeepAlive           time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
}

// DefaultHTTPConfig is suitable for concurrently calling an LLM API.
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		DialTimeout:         5 * time.Second,
		KeepAlive:           30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}
}

// HTTPOption overrides a default HTTPConfig field.
type HTTPOption func(*HTTPConfig)

func WithDialTimeout(value time.Duration) HTTPOption {
	return func(config *HTTPConfig) { config.DialTimeout = value }
}

func WithMaxIdleConnsPerHost(value int) HTTPOption {
	return func(config *HTTPConfig) { config.MaxIdleConnsPerHost = value }
}

func newTransport(config HTTPConfig) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ExpectContinueTimeout: time.Second,
	}
}

// NewHTTPClient returns a client without a whole-request timeout. Long-running
// SSE requests must be bounded by the request context instead.
func NewHTTPClient(options ...HTTPOption) *http.Client {
	config := DefaultHTTPConfig()
	for _, option := range options {
		option(&config)
	}
	return &http.Client{Transport: newTransport(config)}
}

// RetryConfig controls retry behavior. MaxRetries excludes the initial call.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig is a conservative retry policy for transient LLM errors.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   10 * time.Second,
	}
}

// Limiter is satisfied by rate.Limiter and keeps this package dependency-free.
type Limiter interface {
	Wait(context.Context) error
}

// ErrRequestBodyNotReplayable is returned before a retry would send an empty
// POST/PUT body. Use bytes.Reader or strings.Reader when creating requests.
var ErrRequestBodyNotReplayable = errors.New("request body cannot be replayed")

// Client combines connection pooling, optional rate limiting, and retries.
type Client struct {
	http    *http.Client
	retry   RetryConfig
	limiter Limiter
}

// Option configures Client.
type Option func(*Client)

func WithRetry(config RetryConfig) Option {
	return func(client *Client) { client.retry = config }
}

func WithLimiter(limiter Limiter) Option {
	return func(client *Client) { client.limiter = limiter }
}

func WithHTTPOptions(options ...HTTPOption) Option {
	return func(client *Client) { client.http = NewHTTPClient(options...) }
}

// NewClient returns a ready-to-use LLM HTTP client.
func NewClient(options ...Option) *Client {
	client := &Client{
		http:  NewHTTPClient(),
		retry: DefaultRetryConfig(),
	}
	for _, option := range options {
		option(client)
	}
	return client
}

// Do executes req, retrying transient network errors, 429 responses, and 5xx
// responses. Retry waits are interrupted immediately when req.Context ends.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("transport client is not initialized")
	}

	ctx := req.Context()
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	config := normalizedRetryConfig(c.retry)
	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		request, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}

		response, err := c.http.Do(request)
		var wait time.Duration
		switch {
		case err != nil:
			lastErr = err
			wait = backoff(attempt, config)
		case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
			wait = retryAfter(response)
			if wait <= 0 {
				wait = backoff(attempt, config)
			}
			response.Body.Close()
			lastErr = fmt.Errorf("server returned %s", response.Status)
		default:
			return response, nil
		}

		if attempt == config.MaxRetries {
			break
		}
		if !sleep(ctx, wait) {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", config.MaxRetries, lastErr)
}

func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if req.Body == nil {
		return req.Clone(req.Context()), nil
	}
	if req.GetBody == nil {
		if attempt == 0 {
			return req.Clone(req.Context()), nil
		}
		return nil, ErrRequestBodyNotReplayable
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("rebuild request body: %w", err)
	}
	clone := req.Clone(req.Context())
	clone.Body = body
	return clone, nil
}

func normalizedRetryConfig(config RetryConfig) RetryConfig {
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.BaseDelay < 0 {
		config.BaseDelay = 0
	}
	if config.MaxDelay < config.BaseDelay {
		config.MaxDelay = config.BaseDelay
	}
	return config
}

// backoff returns exponential delay with jitter in the 0.9x to 1.1x range.
func backoff(attempt int, config RetryConfig) time.Duration {
	if config.BaseDelay <= 0 {
		return 0
	}

	delay := config.BaseDelay
	for range attempt {
		if delay >= config.MaxDelay/2 {
			delay = config.MaxDelay
			break
		}
		delay *= 2
	}
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}
	if delay < 5 {
		return delay
	}

	// Jitter avoids a thundering herd after a shared provider outage.
	jitter := time.Duration(rand.Int63n(int64(delay) / 5))
	return delay - delay/10 + jitter
}

// retryAfter parses both forms allowed by the Retry-After response header.
func retryAfter(response *http.Response) time.Duration {
	value := response.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
	}
	return 0
}

func sleep(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
