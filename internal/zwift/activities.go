package zwift

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// zwiftTimeLayout is the live listing's own timestamp shape: milliseconds and
// a numeric offset without a colon, which Go's RFC 3339 decoding refuses.
const zwiftTimeLayout = "2006-01-02T15:04:05.000-0700"

// zwiftTime decodes a Zwift activity timestamp in zwiftTimeLayout, falling
// back to RFC 3339 in case Zwift ever emits that instead.
type zwiftTime time.Time

// UnmarshalJSON implements json.Unmarshaler.
func (t *zwiftTime) UnmarshalJSON(raw []byte) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return errors.New("zwift: activity timestamp was not readable")
	}
	parsed, err := time.Parse(zwiftTimeLayout, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, value)
	}
	if err != nil {
		return errors.New("zwift: activity timestamp was not readable")
	}
	*t = zwiftTime(parsed)

	return nil
}

// Activity is the narrow view of one Zwift ride. The response also names every
// other rider who took part; nothing here names them, so they are never decoded.
type Activity struct {
	StartDate        time.Time
	EndDate          time.Time
	Sport            string
	FITBucket        string
	FITKey           string
	ID               int64
	MovingTimeMs     int64
	DistanceMeters   float64
	TotalElevation   float64
	WorldID          int64
	UTCOffsetMinutes int
	PrivateActivity  bool
}

// FITURL returns the public S3 location of this activity's FIT file — no
// bearer token is accepted there — when the listing carried both fields.
func (a *Activity) FITURL() (string, bool) {
	if a.FITBucket == "" || a.FITKey == "" {
		return "", false
	}

	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", a.FITBucket, a.FITKey), true
}

// ActivityDetail is the narrow view of Zwift's single-activity response: the
// structured workout's name, its stable hash and how much of it this ride
// completed. Every other field the response carries -- profile figures and
// the third-party fields the listing also withholds -- is never decoded.
type ActivityDetail struct {
	Name                string
	WorkoutHash         int64
	PercentageCompleted float64
}

// activityDetailDocument is the subset of Zwift's single-activity response
// this package reads.
type activityDetailDocument struct {
	Name                string  `json:"name"`
	WorkoutHash         int64   `json:"workoutHash"`
	PercentageCompleted float64 `json:"percentageCompleted"`
}

// activityDocument is the subset of Zwift's listing entry this package reads.
type activityDocument struct {
	StartDate zwiftTime `json:"startDate"`
	EndDate   zwiftTime `json:"endDate"`
	//nolint:tagliatelle // Zwift's API uses snake_case.
	IDStr            string  `json:"id_str"`
	Sport            string  `json:"sport"`
	FITFileBucket    string  `json:"fitFileBucket"`
	FITFileKey       string  `json:"fitFileKey"`
	MovingTimeInMs   int64   `json:"movingTimeInMs"`
	DistanceInMeters float64 `json:"distanceInMeters"`
	TotalElevation   float64 `json:"totalElevation"`
	WorldID          int64   `json:"worldId"`
	UTCOffsetMinutes int     `json:"utcOffsetMinutes"`
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
		ID:               id,
		StartDate:        time.Time(doc.StartDate),
		EndDate:          time.Time(doc.EndDate),
		MovingTimeMs:     doc.MovingTimeInMs,
		DistanceMeters:   doc.DistanceInMeters,
		TotalElevation:   doc.TotalElevation,
		Sport:            doc.Sport,
		PrivateActivity:  doc.PrivateActivity,
		WorldID:          doc.WorldID,
		FITBucket:        doc.FITFileBucket,
		FITKey:           doc.FITFileKey,
		UTCOffsetMinutes: doc.UTCOffsetMinutes,
	}

	return nil
}

// Summary re-marshals this package's own document — never anything Zwift's
// response carried verbatim — which is what keeps every other rider it named
// out of anything that persists, logs, or notifies from this activity.
func (a *Activity) Summary() ([]byte, error) {
	document := struct {
		StartDate     time.Time `json:"startDate"`
		EndDate       time.Time `json:"endDate"`
		FITFileBucket string    `json:"fitFileBucket"`
		FITFileKey    string    `json:"fitFileKey"`
		//nolint:tagliatelle // Zwift names it id_str, and the decoder above reads that name.
		IDStr            string  `json:"id_str"`
		Sport            string  `json:"sport"`
		MovingTimeInMs   int64   `json:"movingTimeInMs"`
		DistanceInMeters float64 `json:"distanceInMeters"`
		TotalElevation   float64 `json:"totalElevation"`
		WorldID          int64   `json:"worldId"`
		UTCOffsetMinutes int     `json:"utcOffsetMinutes"`
		PrivateActivity  bool    `json:"privateActivity"`
	}{
		IDStr:            strconv.FormatInt(a.ID, 10),
		StartDate:        a.StartDate,
		EndDate:          a.EndDate,
		FITFileBucket:    a.FITBucket,
		FITFileKey:       a.FITKey,
		DistanceInMeters: a.DistanceMeters,
		MovingTimeInMs:   a.MovingTimeMs,
		TotalElevation:   a.TotalElevation,
		WorldID:          a.WorldID,
		UTCOffsetMinutes: a.UTCOffsetMinutes,
		Sport:            a.Sport,
		PrivateActivity:  a.PrivateActivity,
	}

	data, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("zwift: activity summary: %w", err)
	}

	return data, nil
}
