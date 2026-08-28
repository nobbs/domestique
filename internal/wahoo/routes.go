package wahoo

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nobbs/domestique/internal/route"
)

// CreateRoute uploads a FIT course as a Wahoo route owned by Domestique.
func (c *Client) CreateRoute(ctx context.Context, accessToken string, stage *route.Route, fitData []byte) (routeID int64, err error) {
	return c.writeRoute(ctx, http.MethodPost, 0, accessToken, stage, fitData)
}

// UpdateRoute replaces the FIT course and mutable metadata of an owned route.
func (c *Client) UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Route, fitData []byte) (updatedRouteID int64, err error) {
	if routeID <= 0 {
		return 0, errors.New("wahoo: route id must be positive")
	}

	return c.writeRoute(ctx, http.MethodPut, routeID, accessToken, stage, fitData)
}

// ListOwnedRoutes returns every route the account holds that carries an
// external ID, keyed by it. One listing answers what a per-stage lookup used
// to ask once per stage: a library of a hundred stages cost a hundred
// requests against a quota shared by every target, which is what exhausted it.
//
// A route whose external ID this service did not issue was not created here.
// Leaving it out of the map is what keeps a hand-made route unmatchable, and
// therefore undeletable, by everything downstream.
func (c *Client) ListOwnedRoutes(ctx context.Context, accessToken string) (map[string]int64, error) {
	if accessToken == "" {
		return nil, errors.New("wahoo: access token is required")
	}

	request, err := c.newRequest(ctx, http.MethodGet, c.endpoint(c.apiBaseURL, "/v1/routes"), http.NoBody, accessToken)
	if err != nil {
		return nil, err
	}
	var response []routeResponse
	if err := c.doJSON(request, &response); err != nil {
		return nil, err
	}
	if len(response) > maximumRoutes {
		return nil, errors.New("wahoo: route listing exceeded configured bounds")
	}

	owned := make(map[string]int64, len(response))
	for _, item := range response {
		if !route.OwnsExternalID(item.ExternalID) || item.ID <= 0 {
			continue
		}
		// Two routes claiming one external ID leaves no way to say which one
		// this service owns, so every later answer about it — update this,
		// delete that — would be a coin toss silently resolved by map order. A
		// per-stage lookup refused a multiple match for the same reason; the
		// listing has to refuse it too rather than keep whichever arrived last.
		if _, duplicate := owned[item.ExternalID]; duplicate {
			return nil, errors.New("wahoo: route listing contained a duplicate external id")
		}
		owned[item.ExternalID] = item.ID
	}

	return owned, nil
}

// DeleteOwnedRoutes removes every route this service issued the external ID
// for, and reports how many it removed. A route it did not issue is not
// listed, not counted, and not touched.
//
// It reads the account's routes itself rather than taking a list, because the
// two halves have to agree about repeated external IDs. Reconciliation cannot
// act on one — it would not know which route it owns — so its listing refuses
// them outright. Clearing is what an operator reaches for to get out of
// exactly that state, so this sees duplicates and removes every last one.
//
// It waits out a spent request quota rather than stopping at one, because a
// clear is finished only when the target is empty. On a small quota that can
// mean several refills and many minutes; the alternative is an operator
// pressing the same destructive button until the count reaches zero.
//
// A deletion that fails for any other reason ends it, and the count returned
// alongside the error is real: those routes are gone, and repeating the call
// continues from what is left.
func (c *Client) DeleteOwnedRoutes(ctx context.Context, accessToken string) (int, error) {
	if accessToken == "" {
		return 0, errors.New("wahoo: access token is required")
	}
	ctx = withQuotaPatience(ctx)

	request, err := c.newRequest(ctx, http.MethodGet, c.endpoint(c.apiBaseURL, "/v1/routes"), http.NoBody, accessToken)
	if err != nil {
		return 0, err
	}
	var response []routeResponse
	if err := c.doJSON(request, &response); err != nil {
		return 0, err
	}
	if len(response) > maximumRoutes {
		return 0, errors.New("wahoo: route listing exceeded configured bounds")
	}

	deleted := 0
	for _, item := range response {
		if !route.OwnsExternalID(item.ExternalID) || item.ID <= 0 {
			continue
		}
		if err := c.deleteRoute(ctx, item.ID, accessToken); err != nil {
			return deleted, err
		}
		deleted++
	}

	return deleted, nil
}

// DeleteRoute removes one route previously identified as Domestique-owned.
func (c *Client) DeleteRoute(ctx context.Context, routeID int64, accessToken string) error {
	if routeID <= 0 || accessToken == "" {
		return errors.New("wahoo: route id and access token are required")
	}

	return c.deleteRoute(ctx, routeID, accessToken)
}

// deleteRoute is the request itself, without the argument checks a caller
// outside this package needs. DeleteOwnedRoutes uses it for identifiers it
// just read from the API, and to keep its own patient context rather than
// having it replaced.
func (c *Client) deleteRoute(ctx context.Context, routeID int64, accessToken string) error {
	request, err := c.newRequest(ctx, http.MethodDelete, c.endpoint(c.apiBaseURL, fmt.Sprintf("/v1/routes/%d", routeID)), http.NoBody, accessToken)
	if err != nil {
		return err
	}

	return c.doJSON(request, nil)
}

type routeResponse struct {
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	ExternalID string `json:"external_id"`
	ID         int64  `json:"id"`
}

func (c *Client) writeRoute(
	ctx context.Context,
	method string,
	existingRouteID int64,
	accessToken string,
	stage *route.Route,
	fitData []byte,
) (routeID int64, err error) {
	if accessToken == "" || stage == nil || len(fitData) == 0 {
		return 0, errors.New("wahoo: access token, route stage, and fit data are required")
	}

	geometry := stage.Geometry()
	metrics := calculateMetrics(geometry)
	values := url.Values{
		"route[file]":                   {"data:application/vnd.fit;base64," + base64.StdEncoding.EncodeToString(fitData)},
		"route[filename]":               {"domestique.fit"},
		"route[provider_updated_at]":    {stage.Revision()},
		"route[name]":                   {stage.Title()},
		"route[workout_type_family_id]": {"0"},
		"route[start_lat]":              {formatFloat(geometry[0].Latitude)},
		"route[start_lng]":              {formatFloat(geometry[0].Longitude)},
		"route[distance]":               {formatFloat(metrics.distance)},
		"route[ascent]":                 {formatFloat(metrics.ascent)},
		"route[descent]":                {formatFloat(metrics.descent)},
	}
	if method == http.MethodPost {
		values.Set("route[external_id]", stage.Key().ExternalID())
	}

	path := "/v1/routes"
	if existingRouteID > 0 {
		path = fmt.Sprintf("/v1/routes/%d", existingRouteID)
	}
	request, err := c.newRequest(ctx, method, c.endpoint(c.apiBaseURL, path), strings.NewReader(values.Encode()), accessToken)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response routeResponse
	if err := c.doJSON(request, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 || (method == http.MethodPost && response.ExternalID != stage.Key().ExternalID()) {
		return 0, errors.New("wahoo: route response did not contain the expected route")
	}

	return response.ID, nil
}
