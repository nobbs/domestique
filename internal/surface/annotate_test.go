package surface

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/nobbs/domestique/internal/route"
)

// A public endpoint refuses a share of queries under load, and a long stage is
// only classified once every one of its chunk queries lands. Stopping the pass
// at the first failure meant one such stage starved every stage behind it — in
// that run, and in every run after, because the inventory is always walked in
// the same order.
func TestAnnotateClassifiesTheStagesAfterOneThatFailed(t *testing.T) {
	source := &fakeSource{failFor: map[int64]error{2: errors.New("endpoint unavailable")}}
	cache := newFakeCache()
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2, 3))
	if err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	if got, want := classified, 2; got != want {
		t.Errorf("classified = %d, want %d", got, want)
	}
	if got, want := failed, 1; got != want {
		t.Errorf("failed = %d, want %d", got, want)
	}
	if _, stored := cache.stored[3]; !stored {
		t.Error("the stage after the failure was never classified")
	}
}

// Rate limiting is an answer about the server, not about a stage: carrying on
// spends a volunteer's capacity to be refused again.
func TestAnnotateStopsWhenTheEndpointRefusesCapacity(t *testing.T) {
	source := &fakeSource{failFor: map[int64]error{2: ErrRateLimited}}
	cache := newFakeCache()
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2, 3))
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Annotate() error = %v, want ErrRateLimited", err)
	}
	if got, want := classified, 1; got != want {
		t.Errorf("classified = %d, want %d", got, want)
	}
	if got, want := failed, 1; got != want {
		t.Errorf("failed = %d, want %d", got, want)
	}
	if _, stored := cache.stored[3]; stored {
		t.Error("kept asking a server that said it had no capacity")
	}
}

func TestAnnotateSkipsAStageAlreadyClassifiedAgainstItsGeometry(t *testing.T) {
	source := &fakeSource{}
	cache := newFakeCache()
	cache.hashes[1] = "hash-1"
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2))
	if err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	if got, want := classified, 1; got != want {
		t.Errorf("classified = %d, want %d", got, want)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
	if source.asked[1] {
		t.Error("asked the endpoint about a stage already classified for this geometry")
	}
}

func testStages(t *testing.T, routeIDs ...int64) []route.Stage {
	t.Helper()
	stages := make([]route.Stage, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		// Each stage sits at its own latitude, which is how the fake endpoint
		// below tells them apart: it is handed geometry, not identity.
		geometry := []route.Point{
			{Longitude: 8.0, Latitude: float64(routeID)},
			{Longitude: 8.001, Latitude: float64(routeID)},
		}
		stage, err := route.NewStage(
			routeID, 1, "revision", "Route", "Stage", geometry, "hash-"+strconv.FormatInt(routeID, 10),
		)
		if err != nil {
			t.Fatalf("NewStage() error = %v", err)
		}
		stages = append(stages, stage)
	}

	return stages
}

type fakeSource struct {
	failFor map[int64]error
	asked   map[int64]bool
}

func (s *fakeSource) Ways(_ context.Context, points []route.Point) ([]Way, error) {
	if s.asked == nil {
		s.asked = make(map[int64]bool)
	}
	routeID := int64(points[0].Latitude)
	s.asked[routeID] = true
	if err, found := s.failFor[routeID]; found {
		return nil, err
	}

	line := make([]Coordinate, 0, len(points))
	for _, point := range points {
		line = append(line, Coordinate{Longitude: point.Longitude, Latitude: point.Latitude})
	}

	return []Way{{ID: routeID, Kind: KindAsphalt, Line: line}}, nil
}

type fakeCache struct {
	hashes map[int64]string
	stored map[int64][]byte
}

func newFakeCache() *fakeCache {
	return &fakeCache{hashes: make(map[int64]string), stored: make(map[int64][]byte)}
}

func (c *fakeCache) StageSurfaceHash(
	_ context.Context, routeID int64, _ int,
) (contentHash string, found bool, err error) {
	hash, cached := c.hashes[routeID]

	return hash, cached, nil
}

func (c *fakeCache) StoreStageSurface(
	_ context.Context, routeID int64, _ int, contentHash string, ranges []byte, _ float64,
) error {
	c.hashes[routeID] = contentHash
	c.stored[routeID] = ranges

	return nil
}
