package veloplanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRouteCreatesSortedMultiStageTitles(t *testing.T) {
	converted, err := convertRoute(sourceRoute{
		ID:        45,
		Name:      "  Weekend tour  ",
		UpdatedAt: "2026-08-17T07:00:00",
		RouteState: routeState{Stages: []sourceStage{
			{
				Order: 2,
				Name:  "  Return  ",
				Segments: []segment{{Path: &path{DecodedPoints: [][]float64{
					{8.4, 49.0}, {8.5, 49.1},
				}}}},
			},
			{
				Order: 1,
				Name:  "  Outbound  ",
				Segments: []segment{{Path: &path{DecodedPoints: [][]float64{
					{8.2, 48.8}, {8.3, 48.9},
				}}}},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, converted, 2)

	assert.Equal(t, 1, converted[0].Key().StageOrder(), "the stages came back out of order")
	assert.Equal(t, "Weekend tour — Outbound", converted[0].Title())
	assert.Equal(t, "Weekend tour — Return", converted[1].Title())
	assert.NotEqual(t, converted[0].ContentHash(), converted[1].ContentHash(),
		"stage content hashes must differ for different stage content")
}

func TestConvertRouteUsesStableFallbackName(t *testing.T) {
	converted, err := convertRoute(sourceRoute{
		ID:        46,
		UpdatedAt: "2026-08-17T07:00:00",
		RouteState: routeState{Stages: []sourceStage{{
			Order: 1,
			Segments: []segment{{Path: &path{DecodedPoints: [][]float64{
				{8.2, 48.8}, {8.3, 48.9},
			}}}},
		}}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, converted)
	assert.Equal(t, "VeloPlanner 46", converted[0].Title())
}
