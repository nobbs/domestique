package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func tags(pairs ...string) map[string]string {
	values := make(map[string]string, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		values[pairs[index]] = pairs[index+1]
	}

	return values
}

func TestPushedFollowsTheEnginesAccessRules(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		tags   map[string]string
		pushed bool
	}{
		"a footway with no word on bicycles": {tags: tags("highway", "footway"), pushed: true},
		"a footway bicycles may use":         {tags: tags("highway", "footway", "bicycle", "yes")},
		"a pedestrian zone left unsigned":    {tags: tags("highway", "pedestrian")},
		"a way bicycles must dismount on":    {tags: tags("highway", "path", "bicycle", "dismount"), pushed: true},
		"a way bicycles may not use at all":  {tags: tags("highway", "path", "bicycle", "no"), pushed: true},
		"a way nobody may use":               {tags: tags("highway", "track", "access", "no")},
		"a street":                           {tags: tags("highway", "residential")},
		"a cycleway":                         {tags: tags("highway", "cycleway")},
		"a bicycle street":                   {tags: tags("highway", "residential", "bicycle_road", "yes")},
		"a road closed to vehicles":          {tags: tags("highway", "service", "vehicle", "no"), pushed: true},
		"a private footway":                  {tags: tags("highway", "footway", "foot", "private")},
		"a way with no tags":                 {tags: map[string]string{}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.pushed, pushed(testCase.tags))
		})
	}
}

func TestPushingOfFoldsAdjacentPushedWaysIntoOneWindow(t *testing.T) {
	t.Parallel()
	ways := []RoutedWay{
		{EndMetres: 100, Tags: tags("highway", "residential")},
		{EndMetres: 140, Tags: tags("highway", "footway")},
		{EndMetres: 160, Tags: tags("highway", "footway", "surface", "concrete")},
		{EndMetres: 300, Tags: tags("highway", "cycleway")},
		{EndMetres: 320, Tags: tags("highway", "path", "bicycle", "dismount")},
		{EndMetres: 400, Tags: tags("highway", "residential")},
	}

	// The normalised line is twice the engine's length, so every window doubles.
	windows := pushingOf(ways, 800)

	assert.Equal(t, []Window{
		{StartMetres: 200, EndMetres: 320},
		{StartMetres: 600, EndMetres: 640},
	}, windows)
}

func TestPushingOfSaysNothingWithoutWays(t *testing.T) {
	t.Parallel()
	assert.Nil(t, pushingOf(nil, 800))
	assert.Nil(t, pushingOf([]RoutedWay{{EndMetres: 0}}, 800))
	assert.Nil(t, pushingOf([]RoutedWay{{EndMetres: 100, Tags: tags("highway", "footway")}}, 0))
}
