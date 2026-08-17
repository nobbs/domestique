// Package veloplanner reads a private VeloPlanner route library.
package veloplanner

type routeSummary struct {
	ID int64 `json:"id"`
}

type sourceRoute struct {
	Name string `json:"name"`
	//nolint:tagliatelle // VeloPlanner's API uses snake_case.
	UpdatedAt string `json:"updated_at"`
	//nolint:tagliatelle // VeloPlanner's API uses snake_case.
	RouteState routeState `json:"route_state"`
	ID         int64      `json:"id"`
}

type routeState struct {
	Stages []sourceStage `json:"stages"`
}

type sourceStage struct {
	Name     string    `json:"name"`
	Segments []segment `json:"segments"`
	Order    int       `json:"order"`
}

type segment struct {
	StartPointIndex *int  `json:"startPointIndex"`
	EndPointIndex   *int  `json:"endPointIndex"`
	Path            *path `json:"path"`
}

type path struct {
	DecodedPoints [][]float64 `json:"decodedPoints"`
}
