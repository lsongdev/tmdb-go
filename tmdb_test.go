package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memCache struct {
	sync.Mutex
	m map[string][]byte
}

func (c *memCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *memCache) Set(_ context.Context, key string, value []byte) {
	c.Lock()
	defer c.Unlock()
	c.m[key] = value
}

func server(t *testing.T, hits *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		switch {
		case r.Header.Get("Authorization") == "" && q.Get("api_key") != "k3y":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"status_code":7,"status_message":"Invalid API key"}`))
		case r.URL.Path == "/search/movie":
			if q.Get("query") != "Dune" || q.Get("year") != "2021" || q.Get("language") != "zh-CN" {
				t.Errorf("search query = %s", r.URL.RawQuery)
			}
			time.Sleep(20 * time.Millisecond) // let concurrent callers pile up
			w.Write([]byte(`{"page":1,"total_results":1,"results":[{"id":438631,"title":"沙丘","original_title":"Dune","release_date":"2021-09-15"}]}`))
		case r.URL.Path == "/movie/438631":
			w.Write([]byte(`{"id":438631,"title":"Dune","runtime":155,"genres":[{"id":878,"name":"Science Fiction"}]}`))
		case r.URL.Path == "/tv/1399/season/1":
			w.Write([]byte(`{"season_number":1,"episodes":[{"episode_number":1,"name":"Winter Is Coming","still_path":"/s.jpg"}]}`))
		case r.URL.Path == "/tv/1":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
		case r.URL.Path == "/tv/2":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	var hits atomic.Int32
	srv := server(t, &hits)
	cache := &memCache{m: map[string][]byte{}}
	c := New(Options{Key: "k3y", Language: "zh-CN", BaseURL: srv.URL, Cache: cache})

	// Identical concurrent searches share one request.
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := c.SearchMovies(ctx, "Dune", &SearchOptions{Year: 2021})
			if err != nil || len(page.Results) != 1 || page.Results[0].Title != "沙丘" || page.Results[0].Year() != 2021 {
				t.Errorf("search = %+v, %v", page, err)
			}
		}()
	}
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("concurrent searches made %d requests", n)
	}
	// Cached afterwards; the key never contains the API key.
	c.SearchMovies(ctx, "Dune", &SearchOptions{Year: 2021})
	if n := hits.Load(); n != 1 {
		t.Fatalf("cached search made %d requests", n)
	}
	for key := range cache.m {
		if strings.Contains(key, "k3y") {
			t.Fatalf("credentials in cache key %q", key)
		}
	}

	movie, err := c.WithLanguage("en-US").Movie(ctx, 438631)
	if err != nil || movie.Runtime != 155 || movie.Genres[0].Name != "Science Fiction" {
		t.Fatalf("movie = %+v, %v", movie, err)
	}
	season, err := c.Season(ctx, 1399, 1)
	if err != nil || season.Episodes[0].Name != "Winter Is Coming" {
		t.Fatalf("season = %+v, %v", season, err)
	}

	_, err = c.TV(ctx, 1)
	var e *Error
	if !IsNotFound(err) || IsTemporary(err) || !errors.As(err, &e) || e.Code != 34 {
		t.Fatalf("missing show error = %v", err)
	}
	before := hits.Load()
	if _, err := c.TV(ctx, 1); !IsNotFound(err) || hits.Load() != before {
		t.Fatalf("404 not cached: %v", err)
	}
	if _, err := c.TV(ctx, 2); !IsTemporary(err) {
		t.Fatalf("429 should be temporary: %v", err)
	}
	if _, err := New(Options{Key: "bad", BaseURL: srv.URL}).TV(ctx, 3); IsTemporary(err) || IsNotFound(err) {
		t.Fatalf("401 = %v", err)
	}
}

func TestBearerTokenAndNetworkErrors(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization") + "|" + r.URL.Query().Get("api_key")
		w.Write([]byte(`{"id":1}`))
	}))
	c := New(Options{Key: "eyJtoken", BaseURL: srv.URL})
	if _, err := c.TV(context.Background(), 1); err != nil || auth != "Bearer eyJtoken|" {
		t.Fatalf("auth = %q, %v", auth, err)
	}
	srv.Close()
	_, err := New(Options{Key: "s3cret", BaseURL: srv.URL}).TV(context.Background(), 1)
	if !IsTemporary(err) || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("network error = %v", err)
	}
}

func TestDirCache(t *testing.T) {
	ctx := context.Background()
	cache := DirCache{Dir: t.TempDir(), TTL: time.Hour}
	if _, ok := cache.Get(ctx, "/movie/1?language=en"); ok {
		t.Fatal("empty cache hit")
	}
	cache.Set(ctx, "/movie/1?language=en", []byte(`{"id":1}`))
	if data, ok := cache.Get(ctx, "/movie/1?language=en"); !ok || string(data) != `{"id":1}` {
		t.Fatalf("cache = %q, %v", data, ok)
	}
	if _, ok := (DirCache{Dir: cache.Dir, TTL: time.Nanosecond}).Get(ctx, "/movie/1?language=en"); ok {
		t.Fatal("expired entry returned")
	}
}

func TestImageURL(t *testing.T) {
	c := New(Options{})
	if got := c.ImageURL("/a.jpg", "w500"); got != "https://image.tmdb.org/t/p/w500/a.jpg" {
		t.Fatal(got)
	}
	if c.ImageURL("", "w500") != "" {
		t.Fatal("empty path should give empty URL")
	}
}
