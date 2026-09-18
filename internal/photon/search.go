package photon

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// SearchLimit is the most places one search answers.
const SearchLimit = 6

// Kind is what sort of place a search answer is, in the few words the planner
// draws an icon for.
type Kind string

const (
	// KindAddress is a street or a house on one, with no name of its own.
	KindAddress Kind = "address"
	// KindSettlement is a city, town, village or a district of one.
	KindSettlement Kind = "settlement"
	// KindStation is a railway station or halt.
	KindStation Kind = "station"
	// KindPeak is a summit, hill or pass.
	KindPeak Kind = "peak"
	// KindPlace is any other named feature.
	KindPlace Kind = "place"
)

// Place is one search answer: what it is called, where it lies within, and its
// point. Context is empty where the geocoder holds nothing wider.
type Place struct {
	Name      string
	Context   string
	Kind      Kind
	Latitude  float64
	Longitude float64
}

// Near biases a search towards a point without confining it there.
type Near struct {
	Latitude  float64
	Longitude float64
}

// Search answers the places whose name or address matches query, nearest to
// near first where it is given. No match is an empty answer, not a failure.
func (c *Client) Search(ctx context.Context, query string, near *Near) ([]Place, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &Error{Category: FailureRefused}
	}
	values := url.Values{"q": {query}, "limit": {strconv.Itoa(SearchLimit)}}
	if near != nil {
		if math.IsNaN(near.Latitude) || math.IsNaN(near.Longitude) ||
			math.Abs(near.Latitude) > 90 || math.Abs(near.Longitude) > 180 {
			return nil, &Error{Category: FailureRefused}
		}
		values.Set("lat", strconv.FormatFloat(near.Latitude, 'f', -1, 64))
		values.Set("lon", strconv.FormatFloat(near.Longitude, 'f', -1, 64))
	}

	body, status, err := c.get(ctx, "/api", values)
	if err != nil {
		return nil, err
	}

	return parseSearch(body, status)
}

type searchCollection struct {
	Features []struct {
		Properties searchProperties `json:"properties"`
		Geometry   struct {
			// Coordinates is GeoJSON's [longitude, latitude].
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

type searchProperties struct {
	properties

	Country string `json:"country"`
	Type    string `json:"type"`
	//nolint:tagliatelle // Mirrors Photon's own field name.
	OSMKey string `json:"osm_key"`
	//nolint:tagliatelle // Mirrors Photon's own field name.
	OSMValue string `json:"osm_value"`
}

func parseSearch(body []byte, status int) ([]Place, error) {
	var decoded searchCollection
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, &Error{Category: FailureResponse, Status: status}
	}
	places := make([]Place, 0, len(decoded.Features))
	for index := range decoded.Features {
		feature := &decoded.Features[index]
		point := feature.Geometry.Coordinates
		if len(point) < 2 || math.Abs(point[1]) > 90 || math.Abs(point[0]) > 180 {
			return nil, &Error{Category: FailureResponse, Status: status}
		}
		// A search is answered by what was asked for, so a landmark reads by its
		// own name before its address.
		name := firstOf(feature.Properties.Name, what(&feature.Properties.properties), feature.Properties.Country)
		if name == "" {
			continue
		}
		places = append(places, Place{
			Name:      name,
			Context:   within(&feature.Properties, name),
			Kind:      kindOf(&feature.Properties),
			Latitude:  point[1],
			Longitude: point[0],
		})
	}

	return places, nil
}

// within is the wider place an answer lies in: its two nearest named
// enclosures, leaving out any that repeat the answer's own name.
func within(from *searchProperties, name string) string {
	parts := make([]string, 0, 2)
	for _, candidate := range []string{from.District, from.City, from.County, from.State, from.Country} {
		if len(parts) == 2 {
			break
		}
		if candidate == "" || strings.EqualFold(candidate, name) ||
			(len(parts) > 0 && strings.EqualFold(candidate, parts[len(parts)-1])) {
			continue
		}
		parts = append(parts, candidate)
	}

	return strings.Join(parts, ", ")
}

func kindOf(from *searchProperties) Kind {
	switch {
	case from.OSMKey == "railway" && (from.OSMValue == "station" || from.OSMValue == "halt"),
		from.OSMKey == "public_transport" && from.OSMValue == "station":
		return KindStation
	case from.OSMKey == "natural" &&
		(from.OSMValue == "peak" || from.OSMValue == "hill" || from.OSMValue == "volcano" || from.OSMValue == "saddle"):
		return KindPeak
	case from.OSMKey == "place" || from.Type == "city" || from.Type == "district" || from.Type == "locality":
		return KindSettlement
	case from.Name == "" && (from.Type == "house" || from.Type == "street"):
		return KindAddress
	default:
		return KindPlace
	}
}
