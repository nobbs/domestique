package surface

import (
	"context"
	"fmt"

	"github.com/nobbs/domestique/internal/route"
)

// ClassifyGeometry matches a routed geometry against the current map source.
// An empty source generation means that no map is available yet, so the
// geometry remains unclassified without asking the source for ways.
func ClassifyGeometry(ctx context.Context, source Source, points []route.Point) ([]Range, float64, error) {
	if source.Generation() == "" || len(points) == 0 {
		return nil, 0, nil
	}

	ways, err := source.Ways(ctx, points)
	if err != nil {
		return nil, 0, fmt.Errorf("reading ways: %w", err)
	}

	kinds := Match(points, ways)

	return Compress(kinds), MatchedMetres(points, kinds), nil
}
