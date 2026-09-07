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
	"math"
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
	// defaultArchiveBaseURL serves ERA5 reanalysis, which is what a ride older
	// than the forecast endpoint's reach is asked about. It is a different host,
	// not a different path.
	defaultArchiveBaseURL = "https://archive-api.open-meteo.com"
	// defaultTimezone is where every route this service holds is, and what the
	// forecast was asked in before the zone became a setting.
	defaultTimezone = "Europe/Berlin"
	defaultTimeout  = 15 * time.Second
	// maximumBodyBytes bounds a response for the largest request the httpapi
	// boundary allows: 48 points over its 17-day window, roughly 1.2 MB of
	// column-oriented JSON across the eight hourly columns. This leaves
	// headroom without being unbounded.
	maximumBodyBytes = 4 << 20
	hourFormat       = "2006-01-02T15:04"

	// hourlyParams names exactly the series the FIT-course ride window needs.
	// No models parameter: the default is best_match, which is what the
	// parent issue argues for. No unit parameters: the default units are
	// metric, and that is what this service transmits throughout.
	hourlyParams = "temperature_2m,apparent_temperature,precipitation," +
		"precipitation_probability,wind_speed_10m,wind_direction_10m,weather_code,cloud_cover"

	// archiveHourlyParams is the same list without the probability of
	// precipitation, which the reanalysis does not carry: it is a forecast's
	// statement about what might happen, and the archive records what did.
	// Asking for it is refused outright rather than answered with nulls.
	archiveHourlyParams = "temperature_2m,apparent_temperature,precipitation," +
		"wind_speed_10m,wind_direction_10m,weather_code,cloud_cover"

	// forecastPastDays is how far back the forecast endpoint will reach, and so
	// the line between it and the archive. The archive itself runs about five
	// days behind the present, which is why the recent past is asked of the
	// forecast rather than of both.
	forecastPastDays = 92
	dayFormat        = "2006-01-02"
)

// Options configures an Open-Meteo client. There is no API key: the free
// forecast endpoint needs none, so unlike pushover.Options nothing here is a
// secret and nothing here belongs in configuration.
type Options struct {
	Transport http.RoundTripper
	// Now is the clock the choice between the two endpoints is made against.
	// Nil is time.Now.
	Now func() time.Time
	// Timezone reports the IANA zone a forecast is asked and returned in, read
	// again on every request rather than once: an operator editing the setting
	// reaches the next forecast, not the next restart. Nil, or one returning
	// "", is Europe/Berlin, which is where every route this service holds is.
	Timezone func() string
	BaseURL  string
	// ArchiveBaseURL is the reanalysis host, overridden by a test rather than by
	// an operator, exactly as BaseURL is.
	ArchiveBaseURL string
	Timeout        time.Duration
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
	CloudCoverPercent               []float64
}

// Client asks Open-Meteo for an hourly forecast. The host is hardcoded:
// BaseURL exists to be overridden by a test, not by an operator.
type Client struct {
	client     *http.Client
	baseURL    *url.URL
	archiveURL *url.URL
	zone       func() string
	now        func() time.Time
	fallback   *time.Location
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
	archiveBaseURL := options.ArchiveBaseURL
	if archiveBaseURL == "" {
		archiveBaseURL = defaultArchiveBaseURL
	}
	parsedArchiveURL, err := parseOrigin(archiveBaseURL)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: archive base url: %w", err)
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
	// Resolved once here to fail fast at startup on a zone this build cannot load.
	if _, err := resolveLocation(zone(), fallback); err != nil {
		return nil, err
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		baseURL:    parsedBaseURL,
		archiveURL: parsedArchiveURL,
		zone:       zone,
		now:        now,
		fallback:   fallback,
	}, nil
}

func resolveLocation(raw string, fallback *time.Location) (*time.Location, error) {
	if raw == "" {
		return fallback, nil
	}
	location, err := time.LoadLocation(raw)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: loading the %s timezone: %w", raw, err)
	}

	return location, nil
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

	latitudes, longitudes := coordinateColumns(at)

	endpoint := *c.baseURL
	endpoint.Path = "/v1/forecast"
	endpoint.RawQuery = url.Values{
		"latitude":   {latitudes},
		"longitude":  {longitudes},
		"hourly":     {hourlyParams},
		"timezone":   {location.String()},
		"start_hour": {floorHour(from.In(location)).Format(hourFormat)},
		"end_hour":   {ceilHour(to.In(location)).Format(hourFormat)},
	}.Encode()

	return c.fetch(ctx, &endpoint, location, len(at), true)
}

// History returns one hourly series per coordinate for a window that has
// already happened: what the rider actually rode through, rather than what was
// predicted.
//
// A ride inside the forecast endpoint's reach is asked of that endpoint with
// past_days, at model resolution; an older one is asked of the reanalysis
// archive, which is coarser but goes back decades. The archive carries no
// probability of precipitation — it records what fell, not what might have —
// so that one series comes back empty from it.
func (c *Client) History(ctx context.Context, at []Coordinate, from, to time.Time) ([]Hourly, error) {
	if len(at) == 0 {
		return nil, errors.New("openmeteo: at least one coordinate is required")
	}
	if to.Before(from) {
		return nil, errors.New("openmeteo: to must not be before from")
	}
	location, err := resolveLocation(c.zone(), c.fallback)
	if err != nil {
		location = c.fallback
	}
	latitudes, longitudes := coordinateColumns(at)

	if days := c.daysAgo(from, location); days <= forecastPastDays {
		endpoint := *c.baseURL
		endpoint.Path = "/v1/forecast"
		endpoint.RawQuery = url.Values{
			"latitude":  {latitudes},
			"longitude": {longitudes},
			"hourly":    {hourlyParams},
			"timezone":  {location.String()},
			// past_days is what reaches back at all; the hour bounds then narrow
			// the answer to the ride itself. forecast_days is pinned to one
			// rather than left at its default, so a ride from last week does not
			// drag a week of prediction along with it.
			"past_days":     {strconv.Itoa(days)},
			"forecast_days": {"1"},
			"start_hour":    {floorHour(from.In(location)).Format(hourFormat)},
			"end_hour":      {ceilHour(to.In(location)).Format(hourFormat)},
		}.Encode()

		return c.fetch(ctx, &endpoint, location, len(at), true)
	}

	endpoint := *c.archiveURL
	endpoint.Path = "/v1/archive"
	endpoint.RawQuery = url.Values{
		"latitude":  {latitudes},
		"longitude": {longitudes},
		"hourly":    {archiveHourlyParams},
		"timezone":  {location.String()},
		// Whole days: the archive is asked by date, and the caller picks the
		// hours it wants out of what comes back.
		"start_date": {floorHour(from.In(location)).Format(dayFormat)},
		"end_date":   {ceilHour(to.In(location)).Format(dayFormat)},
	}.Encode()

	// The reanalysis is not asked for a probability of precipitation, so it is
	// the one response allowed back without one.
	return c.fetch(ctx, &endpoint, location, len(at), false)
}

// daysAgo is how many days back a moment's own local date is, which is what
// past_days counts: the endpoint reaches back that many whole local days, so an
// elapsed-hours figure would undercount a ride that started late in the day
// before, and ask for a window that does not reach it.
//
// Never negative: a ride recorded in the future, by a device with a wrong
// clock, is asked about as if it were today's.
func (c *Client) daysAgo(at time.Time, location *time.Location) int {
	today := c.now().In(location)
	then := at.In(location)
	days := int(math.Round(
		startOfDay(today).Sub(startOfDay(then)).Hours() / 24))
	if days < 0 {
		return 0
	}

	return days
}

// startOfDay is midnight at the start of a moment's own local day.
func startOfDay(at time.Time) time.Time {
	year, month, day := at.Date()

	return time.Date(year, month, day, 0, 0, 0, 0, at.Location())
}

func coordinateColumns(at []Coordinate) (latitudes, longitudes string) {
	latitude := make([]string, len(at))
	longitude := make([]string, len(at))
	for index, point := range at {
		latitude[index] = strconv.FormatFloat(point.Latitude, 'f', -1, 64)
		longitude[index] = strconv.FormatFloat(point.Longitude, 'f', -1, 64)
	}

	return strings.Join(latitude, ","), strings.Join(longitude, ",")
}

// fetch asks one composed endpoint and decodes one series per coordinate.
func (c *Client) fetch(
	ctx context.Context, endpoint *url.URL, location *time.Location, coordinates int, requireProbability bool,
) (hourlies []Hourly, err error) {
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
	if len(raw) != coordinates {
		return nil, errors.New("openmeteo: response coordinate count did not match the request")
	}

	result := make([]Hourly, len(raw))
	for i := range raw {
		hourly, parseErr := raw[i].Hourly.parse(location, requireProbability)
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
	//nolint:tagliatelle // Mirrors Open-Meteo's own field name.
	CloudCover []float64 `json:"cloud_cover"`
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
//
// requireProbability says whether this response was asked for a probability of
// precipitation. Only a request that did not ask for one may come back without.
func (raw *rawHourly) parse(location *time.Location, requireProbability bool) (Hourly, error) {
	count := len(raw.Time)
	for _, series := range [][]float64{
		raw.Temperature2m, raw.ApparentTemperature, raw.Precipitation,
		raw.WindSpeed10m, raw.WindDirection10m, raw.CloudCover,
	} {
		if len(series) != count {
			return Hourly{}, errors.New("openmeteo: hourly series lengths did not match")
		}
	}
	// Absent only where it was never asked for. The reanalysis does not carry a
	// probability of precipitation and is not asked for one; the forecast
	// endpoint is, and a forecast that came back without it is a response this
	// client should refuse rather than quietly read as "none was recorded".
	if requireProbability || len(raw.PrecipitationProbability) != 0 {
		if len(raw.PrecipitationProbability) != count {
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
		CloudCoverPercent:               raw.CloudCover,
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
