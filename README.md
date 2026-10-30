# tmdb-go

A small Go client for [The Movie Database](https://www.themoviedb.org/) API v3.

- every request takes a `context.Context`; requests time out after 15 s by default
- errors carry the HTTP status: `tmdb.IsNotFound(err)`, `tmdb.IsTemporary(err)` (429, 5xx, network)
- identical concurrent requests share one round trip
- optional response cache: plug in your own store, or use `tmdb.DirCache`
- v3 API keys and v4 read access tokens

```sh
go get github.com/lsongdev/tmdb-go
```

```go
client := tmdb.New(tmdb.Options{
	Key:      os.Getenv("TMDB_KEY"),
	Language: "zh-CN",
	Cache:    tmdb.DirCache{Dir: "tmdb-cache", TTL: 30 * 24 * time.Hour},
})

page, err := client.SearchMovies(ctx, "Dune", &tmdb.SearchOptions{Year: 2021})
movie, err := client.Movie(ctx, page.Results[0].ID)
poster := client.ImageURL(movie.PosterPath, "w500")

show, err := client.TV(ctx, 1396)
season, err := client.Season(ctx, 1396, 1) // all episodes in one request
english := client.WithLanguage("en-US")
```

Available calls: `SearchMovies`, `Movie`, `MovieCredits`, `SearchTV`, `TV`,
`Season`, `Episode`, `TVCredits`. `Movie` and `TV` include alternative titles
(romanizations, regional and former titles) in the same request; use
`AlternativeTitles.All()` to read them for either type.

## Caching

A `Cache` stores raw responses keyed by endpoint and query (language, year,
page; never credentials), so different languages or years never collide.
404 responses are cached as well. Expiry is up to the cache implementation:

```go
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte)
}
```
