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
	// Page is a pointer so a response missing the page object entirely — valid
	// JSON that decodes every field to its zero value — is distinguishable from
	// an explicit, genuinely empty {"number":0,"totalPages":0,"totalElements":0}
	// page. Both would otherwise look identical to a real, empty library.
	Page *pageInfo `json:"page"`
	// Embedded is a pointer, and Tours stays a plain slice deliberately: Go
	// decodes an absent or null "tours" as a nil slice but "tours":[] as a
	// non-nil, zero-length one, so listTours can tell "the listing container
	// was missing" from "the listing container said zero tours" without a
	// second layer of pointers.
	Embedded *embeddedTours `json:"_embedded"` //nolint:tagliatelle // Komoot's API uses HAL's leading underscore.
}

type embeddedTours struct {
	Tours []tourSummary `json:"tours"`
}

// pageInfo's fields are pointers for the same reason coordinate's are: every
// field here is required, and a plain int would let a response missing a
// field — {"page":{}}, still a non-nil Page — decode as page 0 of a 0-element,
// 0-page library and pass every check below as a legitimately empty one.
type pageInfo struct {
	TotalElements *int `json:"totalElements"`
	TotalPages    *int `json:"totalPages"`
	Number        *int `json:"number"`
	Size          *int `json:"size"`
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

// coordinate is one point of a tour's geometry. All three fields are pointers
// so a missing or null field decodes as absent rather than as a fabricated
// zero — a point at (0, 0) with no elevation is real JSON, not a safe
// default. alt is required by Komoot's published schema and was present on
// every point of every planned tour sampled, unlike VeloPlanner's optional
// elevation; convertTour rejects a tour missing any of the three.
type coordinate struct {
	Latitude  *float64 `json:"lat"`
	Longitude *float64 `json:"lng"`
	Elevation *float64 `json:"alt"`
}
