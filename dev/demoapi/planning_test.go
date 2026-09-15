package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

func TestSeedAddsPublishedPlansToTheDemoInventory(t *testing.T) {
	t.Parallel()

	store := demoStore(t)
	planService := plan.NewService(
		planStore{store: store},
		demo.StraightLineRouter{},
		func() time.Time { return time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC) },
		func() (int64, error) { return 42, nil },
	)
	created, err := planService.Create(
		t.Context(), "Demo plan", plan.Gravel,
		[]plan.Waypoint{{Longitude: 8.4, Latitude: 49}, {Longitude: 8.5, Latitude: 49.1}},
	)
	require.NoError(t, err)
	_, err = planService.Replace(
		t.Context(), created.ID, created.Version, created.Name, created.Profile, created.Waypoints, true,
	)
	require.NoError(t, err)

	require.NoError(t, seed(t.Context(), store, []demo.Slot{{ID: demoSubject, State: demo.SlotCurrent}}))
	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err)

	var local []route.Route
	for index := range stages {
		if stages[index].Key().Provider() == route.ProviderLocal {
			local = append(local, stages[index])
		}
	}
	require.Len(t, local, 1)
	assert.Equal(t, created.ID, local[0].Key().SourceRouteID())
}
