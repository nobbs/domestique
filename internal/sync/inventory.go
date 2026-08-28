package sync

import (
	"errors"
	"sort"

	"github.com/nobbs/domestique/internal/route"
)

type targetStage struct {
	sourceRevision string
	contentHash    string
	wahooRouteID   int64
}

type counts struct {
	created int
	updated int
	deleted int
}

func normalizeInventory(stages []route.Route) (map[route.Key]route.Route, []route.Route, error) {
	ordered := append([]route.Route(nil), stages...)
	sort.Slice(ordered, func(left, right int) bool {
		leftKey := ordered[left].Key()
		rightKey := ordered[right].Key()
		if leftKey.Provider() != rightKey.Provider() {
			return leftKey.Provider() < rightKey.Provider()
		}
		if leftKey.SourceRouteID() != rightKey.SourceRouteID() {
			return leftKey.SourceRouteID() < rightKey.SourceRouteID()
		}

		return leftKey.StageOrder() < rightKey.StageOrder()
	})

	desired := make(map[route.Key]route.Route, len(ordered))
	for _, stage := range ordered {
		key := stage.Key()
		if _, exists := desired[key]; exists {
			return nil, nil, errors.New("source inventory contains a duplicate stage")
		}
		desired[key] = stage
	}

	return desired, ordered, nil
}

func missingStages(mappings map[route.Key]targetStage, desired map[route.Key]route.Route) []route.Key {
	missing := make([]route.Key, 0)
	for key := range mappings {
		if _, exists := desired[key]; !exists {
			missing = append(missing, key)
		}
	}
	sort.Slice(missing, func(left, right int) bool {
		if missing[left].Provider() != missing[right].Provider() {
			return missing[left].Provider() < missing[right].Provider()
		}
		if missing[left].SourceRouteID() != missing[right].SourceRouteID() {
			return missing[left].SourceRouteID() < missing[right].SourceRouteID()
		}

		return missing[left].StageOrder() < missing[right].StageOrder()
	})

	return missing
}
