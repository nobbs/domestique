package veloplanner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/nobbs/domestique/internal/route"
)

func convertRoute(source sourceRoute) ([]route.Stage, error) {
	if source.ID <= 0 {
		return nil, errors.New("veloplanner: source route has an invalid id")
	}
	revision := strings.TrimSpace(source.UpdatedAt)
	if revision == "" {
		return nil, fmt.Errorf("veloplanner: route %d has no source revision", source.ID)
	}

	routeName := strings.TrimSpace(source.Name)
	if routeName == "" {
		routeName = fmt.Sprintf("VeloPlanner %d", source.ID)
	}
	multiStage := len(source.RouteState.Stages) > 1
	seenOrders := make(map[int]struct{}, len(source.RouteState.Stages))
	stages := make([]route.Stage, 0, len(source.RouteState.Stages))

	for _, sourceStage := range source.RouteState.Stages {
		if sourceStage.Order <= 0 {
			return nil, fmt.Errorf("veloplanner: route %d has a stage with an invalid order", source.ID)
		}
		if _, exists := seenOrders[sourceStage.Order]; exists {
			return nil, fmt.Errorf("veloplanner: route %d has duplicate stage order", source.ID)
		}
		seenOrders[sourceStage.Order] = struct{}{}

		geometry, err := stageGeometry(sourceStage)
		if err != nil {
			return nil, fmt.Errorf("veloplanner: route %d stage %d: %w", source.ID, sourceStage.Order, err)
		}
		if len(geometry) == 0 {
			continue
		}

		stageName := ""
		if multiStage {
			stageName = strings.TrimSpace(sourceStage.Name)
			if stageName == "" {
				return nil, fmt.Errorf("veloplanner: route %d stage %d has no stage name", source.ID, sourceStage.Order)
			}
		}

		stage, err := route.NewStage(
			route.ProviderVeloPlanner,
			source.ID,
			sourceStage.Order,
			revision,
			routeName,
			stageName,
			geometry,
			"pending",
		)
		if err != nil {
			return nil, fmt.Errorf("veloplanner: route %d stage %d: %w", source.ID, sourceStage.Order, err)
		}
		contentHash, err := stageHash(&stage)
		if err != nil {
			return nil, fmt.Errorf("veloplanner: route %d stage %d: calculating content hash: %w", source.ID, sourceStage.Order, err)
		}
		stage, err = route.NewStage(
			route.ProviderVeloPlanner,
			source.ID,
			sourceStage.Order,
			revision,
			routeName,
			stageName,
			geometry,
			contentHash,
		)
		if err != nil {
			return nil, fmt.Errorf("veloplanner: route %d stage %d: %w", source.ID, sourceStage.Order, err)
		}

		stages = append(stages, stage)
	}

	sort.Slice(stages, func(left, right int) bool {
		return stages[left].Key().StageOrder() < stages[right].Key().StageOrder()
	})

	return stages, nil
}

func stageGeometry(source sourceStage) ([]route.Point, error) {
	segments := slices.Clone(source.Segments)
	sort.SliceStable(segments, func(left, right int) bool {
		if startLeft, startRight := pointIndex(segments[left].StartPointIndex), pointIndex(segments[right].StartPointIndex); startLeft != startRight {
			return startLeft < startRight
		}

		return pointIndex(segments[left].EndPointIndex) < pointIndex(segments[right].EndPointIndex)
	})

	points := make([]route.Point, 0)
	for _, segment := range segments {
		if segment.Path == nil {
			continue
		}
		for _, encodedPoint := range segment.Path.DecodedPoints {
			if len(encodedPoint) != 2 && len(encodedPoint) != 3 {
				return nil, errors.New("decoded point must contain longitude, latitude, and optional elevation")
			}

			point := route.Point{Longitude: encodedPoint[0], Latitude: encodedPoint[1]}
			if len(encodedPoint) == 3 {
				elevation := encodedPoint[2]
				point.Elevation = &elevation
			}
			if pointCount := len(points); pointCount > 0 &&
				points[pointCount-1].Longitude == point.Longitude &&
				points[pointCount-1].Latitude == point.Latitude {
				continue
			}
			points = append(points, point)
		}
	}

	return points, nil
}

func pointIndex(index *int) int {
	if index == nil {
		return 0
	}

	return *index
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
