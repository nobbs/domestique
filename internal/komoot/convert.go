package komoot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nobbs/domestique/internal/route"
)

// tourStageOrder is the only stage order a Komoot tour ever emits. A tour has
// no subdivision, so it maps to exactly one stage: the unit of mirroring
// still applies, it just never repeats within one tour.
const tourStageOrder = 1

func convertTour(tour *tourDetail) (route.Stage, error) {
	if tour.ID <= 0 {
		return route.Stage{}, errors.New("komoot: tour has an invalid id")
	}
	revision := strings.TrimSpace(tour.ChangedAt)
	if revision == "" {
		return route.Stage{}, fmt.Errorf("komoot: tour %d has no source revision", tour.ID)
	}

	routeName := strings.TrimSpace(tour.Name)
	if routeName == "" {
		routeName = fmt.Sprintf("Komoot %d", tour.ID)
	}

	items := tour.Embedded.Coordinates.Items
	geometry := make([]route.Point, 0, len(items))
	for _, item := range items {
		elevation := item.Elevation
		geometry = append(geometry, route.Point{
			Longitude: item.Longitude,
			Latitude:  item.Latitude,
			Elevation: &elevation,
		})
	}

	stage, err := route.NewStage(
		route.ProviderKomoot,
		tour.ID,
		tourStageOrder,
		revision,
		routeName,
		"",
		geometry,
		"pending",
	)
	if err != nil {
		return route.Stage{}, fmt.Errorf("komoot: tour %d: %w", tour.ID, err)
	}

	contentHash, err := stageHash(&stage)
	if err != nil {
		return route.Stage{}, fmt.Errorf("komoot: tour %d: calculating content hash: %w", tour.ID, err)
	}

	stage, err = route.NewStage(
		route.ProviderKomoot,
		tour.ID,
		tourStageOrder,
		revision,
		routeName,
		"",
		geometry,
		contentHash,
	)
	if err != nil {
		return route.Stage{}, fmt.Errorf("komoot: tour %d: %w", tour.ID, err)
	}

	return stage, nil
}

func stageHash(stage *route.Stage) (string, error) {
	geometry := stage.Geometry()
	points := make([]contentHashPoint, 0, len(geometry))
	for _, point := range geometry {
		points = append(points, contentHashPoint{
			Longitude: point.Longitude,
			Latitude:  point.Latitude,
			Elevation: point.Elevation,
		})
	}
	payload := contentHashPayload{
		RouteID:    stage.Key().RouteID(),
		StageOrder: stage.Key().StageOrder(),
		Revision:   stage.Revision(),
		Title:      stage.Title(),
		Geometry:   points,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshalling content hash: %w", err)
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

type contentHashPayload struct {
	Revision   string             `json:"revision"`
	Title      string             `json:"title"`
	Geometry   []contentHashPoint `json:"geometry"`
	RouteID    int64              `json:"routeId"`
	StageOrder int                `json:"stageOrder"`
}

type contentHashPoint struct {
	Elevation *float64 `json:"elevation,omitempty"`
	Longitude float64  `json:"longitude"`
	Latitude  float64  `json:"latitude"`
}
