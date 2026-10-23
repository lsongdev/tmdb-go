package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Page is one page of search results.
type Page[T any] struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []T `json:"results"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Company struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

// Movie is a search result or, with the detail fields filled, a movie
// fetched by ID.
type Movie struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	OriginalLanguage string  `json:"original_language"`
	Overview         string  `json:"overview"`
	ReleaseDate      string  `json:"release_date"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	Popularity       float64 `json:"popularity"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
	Adult            bool    `json:"adult"`
	GenreIDs         []int   `json:"genre_ids,omitempty"`

	// Details
	IMDbID              string    `json:"imdb_id,omitempty"`
	Runtime             int       `json:"runtime,omitempty"`
	Tagline             string    `json:"tagline,omitempty"`
	Status              string    `json:"status,omitempty"`
	Homepage            string    `json:"homepage,omitempty"`
	Budget              int64     `json:"budget,omitempty"`
	Revenue             int64     `json:"revenue,omitempty"`
	Genres              []Genre   `json:"genres,omitempty"`
	ProductionCompanies []Company `json:"production_companies,omitempty"`
	BelongsToCollection *struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		PosterPath   string `json:"poster_path"`
		BackdropPath string `json:"backdrop_path"`
	} `json:"belongs_to_collection,omitempty"`
}

// Year is the release year, or 0 when unknown.
func (m Movie) Year() int { return year(m.ReleaseDate) }

type SearchOptions struct {
	Year         int // release year (movies) or first air year (tv)
	Page         int
	IncludeAdult bool
}

func (o *SearchOptions) values(query, yearKey string) url.Values {
	v := url.Values{"query": {query}}
	if o == nil {
		return v
	}
	if o.Year > 0 {
		v.Set(yearKey, strconv.Itoa(o.Year))
	}
	if o.Page > 1 {
		v.Set("page", strconv.Itoa(o.Page))
	}
	if o.IncludeAdult {
		v.Set("include_adult", "true")
	}
	return v
}

// SearchMovies searches movies by original, translated and alternative titles.
func (c *Client) SearchMovies(ctx context.Context, query string, opts *SearchOptions) (*Page[Movie], error) {
	return fetch[Page[Movie]](ctx, c, "/search/movie", opts.values(query, "year"))
}

// Movie fetches a movie's details.
func (c *Client) Movie(ctx context.Context, id int) (*Movie, error) {
	return fetch[Movie](ctx, c, fmt.Sprintf("/movie/%d", id), nil)
}

type Person struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	OriginalName       string  `json:"original_name"`
	ProfilePath        string  `json:"profile_path"`
	KnownForDepartment string  `json:"known_for_department"`
	Gender             int     `json:"gender"`
	Popularity         float64 `json:"popularity"`
	CreditID           string  `json:"credit_id"`
	Character          string  `json:"character,omitempty"`  // cast
	Order              int     `json:"order,omitempty"`      // cast
	Department         string  `json:"department,omitempty"` // crew
	Job                string  `json:"job,omitempty"`        // crew
}

type Credits struct {
	ID   int      `json:"id"`
	Cast []Person `json:"cast"`
	Crew []Person `json:"crew"`
}

// MovieCredits fetches a movie's cast and crew.
func (c *Client) MovieCredits(ctx context.Context, id int) (*Credits, error) {
	return fetch[Credits](ctx, c, fmt.Sprintf("/movie/%d/credits", id), nil)
}

func year(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}
