// Package tmdb is a small client for The Movie Database API v3.
//
// Every request takes a context, failures carry the HTTP status, identical
// concurrent requests share one round trip, and responses can be kept in a
// pluggable Cache.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL  = "https://api.themoviedb.org/3"
	DefaultImageURL = "https://image.tmdb.org/t/p/"
)

type Options struct {
	// Key is a v3 API key or a v4 read access token (they start with "eyJ").
	Key string
	// Language for localized fields, e.g. "zh-CN". Empty uses TMDB's default.
	Language   string
	BaseURL    string
	ImageURL   string
	HTTPClient *http.Client // defaults to a client with a 15 second timeout
	Cache      Cache        // optional response cache
}

type Client struct {
	opts   Options
	flight *flight
}

func New(opts Options) *Client {
	opts.Key = strings.TrimSpace(opts.Key)
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.ImageURL == "" {
		opts.ImageURL = DefaultImageURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{opts: opts, flight: &flight{calls: map[string]*call{}}}
}

// WithLanguage returns a client for another language sharing the same
// HTTP client, cache and in-flight requests.
func (c *Client) WithLanguage(language string) *Client {
	clone := *c
	clone.opts.Language = language
	return &clone
}

func (c *Client) Language() string { return c.opts.Language }

// ImageURL builds an image URL, e.g. ImageURL("/abc.jpg", "w500").
// Sizes are listed at https://developer.themoviedb.org/docs/image-basics.
func (c *Client) ImageURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	return strings.TrimSuffix(c.opts.ImageURL, "/") + "/" + size + path
}

// Error is a non-2xx response from TMDB.
type Error struct {
	Status  int    `json:"-"`              // HTTP status
	Code    int    `json:"status_code"`    // TMDB status code
	Message string `json:"status_message"` // TMDB status message
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("tmdb: HTTP %d: %s (code %d)", e.Status, e.Message, e.Code)
	}
	return fmt.Sprintf("tmdb: HTTP %d", e.Status)
}

// IsNotFound reports whether TMDB has no such resource.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// IsTemporary reports failures worth retrying later: rate limiting, server
// errors and network failures, but not cancellation or client errors.
func IsTemporary(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Status == http.StatusTooManyRequests || e.Status >= 500
	}
	var u *url.Error
	return errors.As(err, &u)
}

// get fetches an endpoint and decodes its JSON body into out.
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	if c.opts.Language != "" && query.Get("language") == "" {
		query.Set("language", c.opts.Language)
	}
	key := endpoint + "?" + query.Encode() // never contains credentials
	data, err := c.flight.do(key, func() ([]byte, error) { return c.fetch(ctx, key, endpoint, query) })
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("tmdb: decode %s: %w", endpoint, err)
	}
	return nil
}

// fetch gets and decodes one endpoint.
func fetch[T any](ctx context.Context, c *Client, endpoint string, query url.Values) (*T, error) {
	var v T
	if err := c.get(ctx, endpoint, query, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// notFound marks a cached 404 so missing items are not requested again.
var notFound = []byte("404")

func (c *Client) fetch(ctx context.Context, key, endpoint string, query url.Values) ([]byte, error) {
	if c.opts.Cache != nil {
		if data, ok := c.opts.Cache.Get(ctx, key); ok {
			if string(data) == string(notFound) {
				return nil, &Error{Status: http.StatusNotFound}
			}
			return data, nil
		}
	}
	bearer := strings.HasPrefix(c.opts.Key, "eyJ")
	if c.opts.Key != "" && !bearer {
		query = cloneValues(query)
		query.Set("api_key", c.opts.Key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.BaseURL+endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+c.opts.Key)
	}
	res, err := c.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, redact(err, c.opts.Key)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, redact(err, c.opts.Key)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		e := &Error{Status: res.StatusCode}
		_ = json.Unmarshal(data, e)
		if e.Status == http.StatusNotFound && c.opts.Cache != nil {
			c.opts.Cache.Set(ctx, key, notFound)
		}
		return nil, e
	}
	if c.opts.Cache != nil {
		c.opts.Cache.Set(ctx, key, data)
	}
	return data, nil
}

// redact keeps the API key out of errors that echo the request URL.
func redact(err error, key string) error {
	var u *url.Error
	if key != "" && errors.As(err, &u) {
		u.URL = strings.ReplaceAll(u.URL, key, "REDACTED")
	}
	return err
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, values := range v {
		out[k] = append([]string(nil), values...)
	}
	return out
}

// flight shares one in-flight request between identical concurrent calls,
// e.g. many episodes of one show looking up the same season.
type flight struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	done chan struct{}
	data []byte
	err  error
}

func (f *flight) do(key string, fn func() ([]byte, error)) ([]byte, error) {
	f.mu.Lock()
	if c, ok := f.calls[key]; ok {
		f.mu.Unlock()
		<-c.done
		return c.data, c.err
	}
	c := &call{done: make(chan struct{})}
	f.calls[key] = c
	f.mu.Unlock()
	c.data, c.err = fn()
	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()
	close(c.done)
	return c.data, c.err
}
