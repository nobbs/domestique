package zwift_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/zwift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorldByIDNamesAKnownWorld(t *testing.T) {
	t.Parallel()

	world, found := zwift.WorldByID(9)

	require.True(t, found, "world 9 is in the table")
	assert.Equal(t, "Makuri Islands", world.Name)
	assert.InDelta(t, -10.73746, world.North, 1e-9)
	assert.InDelta(t, 165.88222, world.East, 1e-9)
	assert.Equal(t, "MiniMap_Japan.png", world.ImageFile)
}

func TestWorldByIDRefusesAWorldTheTableDoesNotName(t *testing.T) {
	t.Parallel()

	// Zwift has no world 12; nothing in the table may invent one.
	_, found := zwift.WorldByID(12)

	assert.False(t, found)
}

// Every id Zwift issues has an entry, and each entry's box runs north-west to
// south-east — the direction the transform that places a ride on it assumes.
func TestEveryWorldIsBoxedNorthWestToSouthEast(t *testing.T) {
	t.Parallel()

	for id := int64(1); id <= 13; id++ {
		world, found := zwift.WorldByID(id)
		if id == 12 {
			assert.False(t, found, "Zwift has no world 12")

			continue
		}
		require.Truef(t, found, "world %d", id)
		assert.Greaterf(t, world.North, world.South, "world %d spans north to south", id)
		assert.Lessf(t, world.West, world.East, "world %d spans west to east", id)
		assert.NotEmptyf(t, world.ImageFile, "world %d names its artwork", id)
	}
}

func TestWorldOfReadsTheStoredSummary(t *testing.T) {
	t.Parallel()

	activity := zwift.Activity{ID: 42, WorldID: 1}
	summary, err := activity.Summary()
	require.NoError(t, err, "Summary()")

	world, found := zwift.WorldOf(summary)

	require.True(t, found)
	assert.Equal(t, "Watopia", world.Name)
}

func TestWorldOfHasNoWorldForASummaryWithout(t *testing.T) {
	t.Parallel()

	for name, summary := range map[string][]byte{
		"no world":      []byte(`{"sport":"CYCLING"}`),
		"unknown world": []byte(`{"worldId":99}`),
		"not a summary": []byte("not json"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, found := zwift.WorldOf(summary)

			assert.False(t, found)
		})
	}
}
