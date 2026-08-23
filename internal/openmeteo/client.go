// Package openmeteo asks Open-Meteo for an hourly forecast at a list of
// coordinates over one shared time window. It owns no route logic and no
// rendering: it takes coordinates and a time window, and returns hourly
// series.
package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	// Open-Meteo is always asked in Europe/Berlin local time, so this
	// service's own timezone database must be complete regardless of what the
	// runtime image carries. The hardened base image is not guaranteed to
	// ship one.
	_ "time/tzdata"
)

const (
	defaultBaseURL   = "https://api.open-meteo.com"
	defaultTimeout   = 15 * time.Second
	maximumBodyBytes = 1 << 20
	hourFormat       = "2006-01-02T15:04"

	// hourlyParams names exactly the series the FIT-course ride window needs.
	// No models parameter: the default is best_match, which is what the
	// parent issue argues for. No unit parameters: the default units are
	// metric, and that is what this service transmits throughout.
	hourlyParams = "temperature_2m,apparent_temperature,precipitation," +
		"precipitation_probability,wind_speed_10m,wind_direction_10m,weather_code"
)

// berlin is loaded once. Every route this service holds is in Germany, and a
// fixed zone keeps a returned timestamp describing where the rider reads it
// rather than where the route happens to be.
//
//nolint:gochecknoglobals // Every request needs the same fixed zone; loaded once rather than reloaded per call.
var berlin = func() *time.Location {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		// Unreachable with time/tzdata embedded above; a panic here would mean
		// the standard library's own embedded database is corrupt.
		panic("openmeteo: Europe/Berlin timezone data is unavailable: " + err.Error())
	}

	return location
}()

// Options configures an Open-Meteo client. There is no API key: the free
// forecast endpoint needs none, so unlike pushover.Options nothing here is a
// secret and nothing here belongs in configuration.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Timeout   time.Duration
}

// Coordinate is one point Forecast asks about.
type Coordinate struct {
	Latitude, Longitude float64
}

// Hourly is one coordinate's forecast series, column-oriented the way
// Open-Meteo returns it: index i across every slice describes the same hour.
type Hourly struct {
	Time                            []time.Time
	TemperatureCelsius              []float64
	ApparentTemperatureCelsius      []float64
	PrecipitationMillimetres        []float64
	PrecipitationProbabilityPercent []float64
	WindSpeedKMH                    []float64
	WindDirectionDegrees            []float64
	WeatherCode                     []int
}

// Client asks Open-Meteo for an hourly forecast. The host is hardcoded:
// BaseURL exists to be overridden by a test, not by an operator.
type Client struct {
	client  *http.Client
	baseURL *url.URL
}

// New creates an Open-Meteo client without contacting the upstream service.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("openmeteo: options are required")
	}
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsedBaseURL, err := parseOrigin(baseURL)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("openmeteo: timeout must be positive")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		baseURL: parsedBaseURL,
	}, nil
}

// Forecast returns one hourly series per coordinate, in the order given,
// spanning from..to. from and to are truncated to minute precision in
// Europe/Berlin local time, matching Open-Meteo's start_hour/end_hour
// granularity.
//
// Open-Meteo replies with a JSON array, one object per coordinate, for
// several coordinates — but a bare object, not a one-element array, for
// exactly one. Both are handled here.
func (c *Client) Forecast(ctx context.Context, at []Coordinate, from, to time.Time) (hourlies []Hourly, err error) {
	if len(at) == 0 {
		return nil, errors.New("openmeteo: at least one coordinate is required")
	}
	if to.Before(from) {
		return nil, errors.New("openmeteo: to must not be before from")
	}

	latitudes := make([]string, len(at))
	longitudes := make([]string, len(at))
	for i, point := range at {
		latitudes[i] = strconv.FormatFloat(point.Latitude, 'f', -1, 64)
		longitudes[i] = strconv.FormatFloat(point.Longitude, 'f', -1, 64)
	}

	endpoint := *c.baseURL
	endpoint.Path = "/v1/forecast"
	endpoint.RawQuery = url.Values{
		"latitude":   {strings.Join(latitudes, ",")},
		"longitude":  {strings.Join(longitudes, ",")},
		"hourly":     {hourlyParams},
		"timezone":   {"Europe/Berlin"},
		"start_hour": {from.In(berlin).Format(hourFormat)},
		"end_hour":   {to.In(berlin).Format(hourFormat)},
	}.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return nil, errors.New("openmeteo: request failed")
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()
	// Checked before the body is read: a provider failure must not put the
	// provider's response text into the returned error, and there is nothing
	// in a failure body this client has any use for.
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("openmeteo: request returned HTTP %d", response.StatusCode)
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return nil, errors.New("openmeteo: response could not be read")
	}
	if len(body) > maximumBodyBytes {
		return nil, errors.New("openmeteo: response exceeded size limit")
	}

	raw, decodeErr := decodeForecastResponse(body)
	if decodeErr != nil {
		return nil, decodeErr
	}
	if len(raw) != len(at) {
		return nil, errors.New("openmeteo: response coordinate count did not match the request")
	}

	result := make([]Hourly, len(raw))
	for i := range raw {
		hourly, parseErr := raw[i].Hourly.parse()
		if parseErr != nil {
			return nil, parseErr
		}
		result[i] = hourly
	}

	return result, nil
}

// rawForecastResponse is one coordinate's response as Open-Meteo shapes it.
type rawForecastResponse struct {
	Hourly rawHourly `json:"hourly"`
}

type rawHourly struct {
	Time          []string  `json:"time"`
	Precipitation []float64 `json:"precipitation"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	Temperature2m []float64 `json:"temperature_2m"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	ApparentTemperature []float64 `json:"apparent_temperature"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	PrecipitationProbability []float64 `json:"precipitation_probability"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	WindSpeed10m []float64 `json:"wind_speed_10m"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	WindDirection10m []float64 `json:"wind_direction_10m"`
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	WeatherCode []int `json:"weather_code"`
}

// decodeForecastResponse handles both response shapes: a bare object for one
// coordinate, an array for several.
func decodeForecastResponse(body []byte) ([]rawForecastResponse, error) {
	var series []rawForecastResponse
	if err := json.Unmarshal(body, &series); err == nil {
		return series, nil
	}
	var single rawForecastResponse
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, errors.New("openmeteo: response could not be decoded")
	}

	return []rawForecastResponse{single}, nil
}

// parse converts one coordinate's raw hourly block, validating that every
// series is the same length as the timestamps naming them.
func (raw *rawHourly) parse() (Hourly, error) {
	count := len(raw.Time)
	for _, series := range [][]float64{
		raw.Temperature2m, raw.ApparentTemperature, raw.Precipitation, raw.PrecipitationProbability,
		raw.WindSpeed10m, raw.WindDirection10m,
	} {
		if len(series) != count {
			return Hourly{}, errors.New("openmeteo: hourly series lengths did not match")
		}
	}
	if len(raw.WeatherCode) != count {
		return Hourly{}, errors.New("openmeteo: hourly series lengths did not match")
	}

	times := make([]time.Time, count)
	for i, value := range raw.Time {
		parsed, err := time.ParseInLocation(hourFormat, value, berlin)
		if err != nil {
			return Hourly{}, fmt.Errorf("openmeteo: parsing hourly time: %w", err)
		}
		times[i] = parsed
	}

	return Hourly{
		Time:                            times,
		TemperatureCelsius:              raw.Temperature2m,
		ApparentTemperatureCelsius:      raw.ApparentTemperature,
		PrecipitationMillimetres:        raw.Precipitation,
		PrecipitationProbabilityPercent: raw.PrecipitationProbability,
		WindSpeedKMH:                    raw.WindSpeed10m,
		WindDirectionDegrees:            raw.WindDirection10m,
		WeatherCode:                     raw.WeatherCode,
	}, nil
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
