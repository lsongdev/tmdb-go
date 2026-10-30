package tmdb

import (
	"context"
	"fmt"
	"net/url"
)

// TV is a search result or, with the detail fields filled, a show fetched
// by ID.
type TV struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	OriginalName     string   `json:"original_name"`
	OriginalLanguage string   `json:"original_language"`
	OriginCountry    []string `json:"origin_country"`
	Overview         string   `json:"overview"`
	FirstAirDate     string   `json:"first_air_date"`
	PosterPath       string   `json:"poster_path"`
	BackdropPath     string   `json:"backdrop_path"`
	Popularity       float64  `json:"popularity"`
	VoteAverage      float64  `json:"vote_average"`
	VoteCount        int      `json:"vote_count"`
	Adult            bool     `json:"adult"`
	GenreIDs         []int    `json:"genre_ids,omitempty"`

	// Details
	LastAirDate      string    `json:"last_air_date,omitempty"`
	Status           string    `json:"status,omitempty"`
	Tagline          string    `json:"tagline,omitempty"`
	Type             string    `json:"type,omitempty"`
	Homepage         string    `json:"homepage,omitempty"`
	InProduction     bool      `json:"in_production,omitempty"`
	NumberOfSeasons  int       `json:"number_of_seasons,omitempty"`
	NumberOfEpisodes int       `json:"number_of_episodes,omitempty"`
	EpisodeRunTime   []int     `json:"episode_run_time,omitempty"`
	Genres           []Genre   `json:"genres,omitempty"`
	Networks         []Company `json:"networks,omitempty"`
	CreatedBy        []Person  `json:"created_by,omitempty"`
	Seasons          []Season  `json:"seasons,omitempty"`

	AlternativeTitles *AlternativeTitles `json:"alternative_titles,omitempty"`
}

// Year is the first air year, or 0 when unknown.
func (t TV) Year() int { return year(t.FirstAirDate) }

type Season struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Overview     string    `json:"overview"`
	SeasonNumber int       `json:"season_number"`
	AirDate      string    `json:"air_date"`
	PosterPath   string    `json:"poster_path"`
	VoteAverage  float64   `json:"vote_average"`
	EpisodeCount int       `json:"episode_count,omitempty"` // in TV.Seasons
	Episodes     []Episode `json:"episodes,omitempty"`      // from Client.Season
}

type Episode struct {
	ID             int      `json:"id"`
	ShowID         int      `json:"show_id"`
	Name           string   `json:"name"`
	Overview       string   `json:"overview"`
	SeasonNumber   int      `json:"season_number"`
	EpisodeNumber  int      `json:"episode_number"`
	AirDate        string   `json:"air_date"`
	Runtime        int      `json:"runtime"`
	StillPath      string   `json:"still_path"`
	VoteAverage    float64  `json:"vote_average"`
	VoteCount      int      `json:"vote_count"`
	ProductionCode string   `json:"production_code"`
	Crew           []Person `json:"crew,omitempty"`
	GuestStars     []Person `json:"guest_stars,omitempty"`
}

// SearchTV searches shows by original, translated and also-known-as names.
func (c *Client) SearchTV(ctx context.Context, query string, opts *SearchOptions) (*Page[TV], error) {
	return fetch[Page[TV]](ctx, c, "/search/tv", opts.values(query, "first_air_date_year"))
}

// TV fetches a show's details, including its seasons and alternative titles.
func (c *Client) TV(ctx context.Context, id int) (*TV, error) {
	return fetch[TV](ctx, c, fmt.Sprintf("/tv/%d", id), url.Values{"append_to_response": {"alternative_titles"}})
}

// Season fetches a season with all of its episodes in one request.
func (c *Client) Season(ctx context.Context, show, season int) (*Season, error) {
	return fetch[Season](ctx, c, fmt.Sprintf("/tv/%d/season/%d", show, season), nil)
}

// Episode fetches a single episode.
func (c *Client) Episode(ctx context.Context, show, season, episode int) (*Episode, error) {
	return fetch[Episode](ctx, c, fmt.Sprintf("/tv/%d/season/%d/episode/%d", show, season, episode), nil)
}

// TVCredits fetches the cast and crew of a show's latest season.
func (c *Client) TVCredits(ctx context.Context, id int) (*Credits, error) {
	return fetch[Credits](ctx, c, fmt.Sprintf("/tv/%d/credits", id), nil)
}
