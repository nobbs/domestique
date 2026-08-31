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

	// Open-Meteo is asked in the service's own local time, so this service's
	// timezone database must be complete regardless of what the runtime image
	// carries. The hardened base image is not guaranteed to ship one.
	_ "time/tzdata"
)

const (
	defaultBaseURL = "https://api.open-meteo.com"
	// defaultTimezone is where every route this service holds is, and what the
	// forecast was asked in before the zone became a setting.
	defaultTimezone = "Europe/Berlin"
	defaultTimeout  = 15 * time.Second
	// maximumBodyBytes bounds a response for the largest request the httpapi
	// boundary allows: 48 points over its 17-day window, roughly 1.05 MB of
	// column-oriented JSON. This leaves headroom without being unbounded.
	maximumBodyBytes = 4 << 20
	hourFormat       = "2006-01-02T15:04"

	// hourlyParams names exactly the series the FIT-course ride window needs.
	// No models parameter: the default is best_match, which is what the
	// parent issue argues for. No unit parameters: the default units are
	// metric, and that is what this service transmits throughout.
	hourlyParams = "temperature_2m,apparent_temperature,precipitation," +
		"precipitation_probability,wind_speed_10m,wind_direction_10m,weather_code"
)

// Options configures an Open-Meteo client. There is no API key: the free
// forecast endpoint needs none, so unlike pushover.Options nothing here is a
// secret and nothing here belongs in configuration.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	// Timezone reports the IANA zone a forecast is asked and returned in, read
	// again on every request rather than once: an operator editing the setting
	// reaches the next forecast, not the next restart. Nil, or one returning
	// "", is Europe/Berlin, which is where every route this service holds is.
	Timezone func() string
	Timeout  time.Duration
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
	client   *http.Client
	baseURL  *url.URL
	zone     func() string
	fallback *time.Location
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
	fallback, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: loading the default %s timezone: %w", defaultTimezone, err)
	}
	zone := options.Timezone
	if zone == nil {
		zone = func() string { return "" }
	}
	// Resolved once here to fail fast at startup on a zone this build cannot
	// load. Every actual request resolves again, read fresh: a settings edit
	// reaches the next forecast rather than the next restart.
	if _, err := resolveLocation(zone(), fallback); err != nil {
		return nil, err
	}

	return &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		baseURL:  parsedBaseURL,
		zone:     zone,
		fallback: fallback,
	}, nil
}

// resolveLocation loads the named zone, or reports the fallback for an empty
// one.
func resolveLocation(raw string, fallback *time.Location) (*time.Location, error) {
	if raw == "" {
		return fallback, nil
	}

	return time.LoadLocation(raw)
}

// Forecast returns one hourly series per coordinate, in the order given, spanning
// from..to. from is rounded down and to rounded up to the nearest local hour in
// the service's zone, so a window that does not land on the hour still gets every hour a
// point could resolve to. Open-Meteo replies with an array for several
// coordinates and a bare object for exactly one; both are handled.
func (c *Client) Forecast(ctx context.Context, at []Coordinate, from, to time.Time) (hourlies []Hourly, err error) {
	if len(at) == 0 {
		return nil, errors.New("openmeteo: at least one coordinate is required")
	}
	if to.Before(from) {
		return nil, errors.New("openmeteo: to must not be before from")
	}
	location, err := resolveLocation(c.zone(), c.fallback)
	if err != nil {
		// A live setting was validated when it was written, so a load failure
		// here is the tzdata database changing under a running process. A
		// forecast in a stale zone is a smaller problem than a forecast that
		// stopped working, so this falls back rather than failing the request.
		location = c.fallback
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
		"timezone":   {location.String()},
		"start_hour": {floorHour(from.In(location)).Format(hourFormat)},
		"end_hour":   {ceilHour(to.In(location)).Format(hourFormat)},
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
		hourly, parseErr := raw[i].Hourly.parse(location)
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
func (raw *rawHourly) parse(location *time.Location) (Hourly, error) {
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
		parsed, err := time.ParseInLocation(hourFormat, value, location)
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

// floorHour rounds t down to the start of its hour.
func floorHour(t time.Time) time.Time {
	return t.Truncate(time.Hour)
}

// ceilHour rounds t up to the start of the next hour, or leaves it alone when
// it already lands on one.
func ceilHour(t time.Time) time.Time {
	floored := floorHour(t)
	if floored.Equal(t) {
		return floored
	}

	return floored.Add(time.Hour)
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
