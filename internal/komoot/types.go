// Package komoot reads a private Komoot route library.
package komoot

// accountResponse is the unofficial v006 login response. The field names are
// misleading: username carries the numeric user id, and password carries a
// session token rather than the account password.
type accountResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type toursResponse struct {
	Embedded struct {
		Tours []tourSummary `json:"tours"`
	} `json:"_embedded"` //nolint:tagliatelle // Komoot's API uses HAL's leading underscore.
	Page pageInfo `json:"page"`
}

type pageInfo struct {
	Size          int `json:"size"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
	Number        int `json:"number"`
}

type tourSummary struct {
	ID int64 `json:"id"`
}

type tourDetail struct {
	Type string `json:"type"`
	Name string `json:"name"`
	//nolint:tagliatelle // Komoot's API uses snake_case.
	ChangedAt string `json:"changed_at"`
	Embedded  struct {
		Coordinates struct {
			Items []coordinate `json:"items"`
		} `json:"coordinates"`
	} `json:"_embedded"` //nolint:tagliatelle // Komoot's API uses HAL's leading underscore.
	ID int64 `json:"id"`
}

// coordinate is one point of a tour's geometry. alt is required by Komoot's
// published schema and was present on every point of every planned tour
// sampled, unlike VeloPlanner's optional elevation.
type coordinate struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Elevation float64 `json:"alt"`
}
