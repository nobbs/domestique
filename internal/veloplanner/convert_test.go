package veloplanner

import "testing"

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
	if err != nil {
		t.Fatalf("convertRoute() error = %v", err)
	}
	if got, want := len(converted), 2; got != want {
		t.Fatalf("len(convertRoute()) = %d, want %d", got, want)
	}

	if got, want := converted[0].Key().StageOrder(), 1; got != want {
		t.Errorf("first stage order = %d, want %d", got, want)
	}
	if got, want := converted[0].Title(), "Weekend tour — Outbound"; got != want {
		t.Errorf("first title = %q, want %q", got, want)
	}
	if got, want := converted[1].Title(), "Weekend tour — Return"; got != want {
		t.Errorf("second title = %q, want %q", got, want)
	}
	if converted[0].ContentHash() == converted[1].ContentHash() {
		t.Error("stage content hashes must differ for different stage content")
	}
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
	if err != nil {
		t.Fatalf("convertRoute() error = %v", err)
	}
	if got, want := converted[0].Title(), "VeloPlanner 46"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
}
