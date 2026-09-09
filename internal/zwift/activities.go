package zwift

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Activity is the narrow view of one Zwift ride. The response also names every
// other rider who took part; nothing here names them, so they are never decoded.
type Activity struct {
	StartDate       time.Time
	EndDate         time.Time
	Sport           string
	FitnessStatus   string
	FullDataURL     string
	ID              int64
	MovingTimeMs    int64
	DistanceMeters  float64
	TotalElevation  float64
	WorldID         int64
	PrivateActivity bool
}

// activityDocument is the subset of Zwift's JSON this package reads, whether
// it arrived as a listing entry or the single-activity response.
type activityDocument struct {
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	//nolint:tagliatelle // Zwift's API uses snake_case.
	IDStr       string `json:"id_str"`
	Sport       string `json:"sport"`
	FitnessData struct {
		Status      string `json:"status"`
		FullDataURL string `json:"fullDataUrl"`
	} `json:"fitnessData"`
	MovingTimeInMs   int64   `json:"movingTimeInMs"`
	DistanceInMeters float64 `json:"distanceInMeters"`
	TotalElevation   float64 `json:"totalElevation"`
	WorldID          int64   `json:"worldId"`
	PrivateActivity  bool    `json:"privateActivity"`
}

// UnmarshalJSON reads one Zwift activity document, taking the id from id_str:
// the numeric "id" field is float-rounded by Zwift's own JSON encoder for an
// id this large and is never decoded.
func (a *Activity) UnmarshalJSON(raw []byte) error {
	var doc activityDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("zwift: activity: %w", err)
	}
	id, err := strconv.ParseInt(doc.IDStr, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("zwift: activity id_str was not a valid id")
	}

	*a = Activity{
		ID:              id,
		StartDate:       doc.StartDate,
		EndDate:         doc.EndDate,
		MovingTimeMs:    doc.MovingTimeInMs,
		DistanceMeters:  doc.DistanceInMeters,
		TotalElevation:  doc.TotalElevation,
		Sport:           doc.Sport,
		PrivateActivity: doc.PrivateActivity,
		WorldID:         doc.WorldID,
		FitnessStatus:   doc.FitnessData.Status,
		FullDataURL:     doc.FitnessData.FullDataURL,
	}

	return nil
}

// Summary re-marshals this package's own document — never anything Zwift's
// response carried verbatim — which is what keeps every other rider it named
// out of anything that persists, logs, or notifies from this activity.
func (a *Activity) Summary() ([]byte, error) {
	document := struct {
		StartDate        time.Time `json:"startDate"`
		EndDate          time.Time `json:"endDate"`
		FullDataURL      string    `json:"fullDataUrl"`
		ID               int64     `json:"id"`
		MovingTimeInMs   int64     `json:"movingTimeInMs"`
		DistanceInMeters float64   `json:"distanceInMeters"`
		TotalElevation   float64   `json:"totalElevation"`
		WorldID          int64     `json:"worldId"`
	}{
		ID:               a.ID,
		StartDate:        a.StartDate,
		EndDate:          a.EndDate,
		FullDataURL:      a.FullDataURL,
		DistanceInMeters: a.DistanceMeters,
		MovingTimeInMs:   a.MovingTimeMs,
		TotalElevation:   a.TotalElevation,
		WorldID:          a.WorldID,
	}

	data, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("zwift: activity summary: %w", err)
	}

	return data, nil
}
