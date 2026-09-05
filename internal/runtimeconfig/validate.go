package runtimeconfig

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	// A zone this binary cannot load is a zone an operator cannot choose, so the
	// database travels with the binary rather than depending on what the runtime
	// image happens to carry. The hardened base image ships none.
	_ "time/tzdata"
)

// minimumInterval is the floor under every duration here. These are stored as
// whole seconds, so anything shorter is written as zero and then refused.
const minimumInterval = time.Second

// Validate checks every rule and returns the normalised settings: lists trimmed
// of whitespace and repeats, so what is stored is exactly what was checked.
//
//nolint:gocritic // value receiver: validation returns a normalised copy rather than editing the caller's.
func (v Values) Validate() (Values, error) {
	timezone, err := ValidateTimezone(v.Timezone)
	if err != nil {
		return Values{}, err
	}
	if syncErr := ValidateSync(v.Sync); syncErr != nil {
		return Values{}, syncErr
	}
	notifications, err := ValidateNotifications(v.Notifications)
	if err != nil {
		return Values{}, err
	}

	basemaps, err := ValidateBasemaps(v.Basemaps)
	if err != nil {
		return Values{}, err
	}

	surface, err := ValidateSurface(v.Surface)
	if err != nil {
		return Values{}, err
	}

	wahoo, err := ValidateWahoo(v.Wahoo)
	if err != nil {
		return Values{}, err
	}

	sources, err := ValidateSources(v.Sources)
	if err != nil {
		return Values{}, err
	}

	v.Timezone = timezone
	v.Notifications = notifications
	v.Basemaps = basemaps
	v.Surface = surface
	v.Wahoo = wahoo
	v.Sources = sources

	return v, nil
}

// ValidateTimezone rejects a zone this binary cannot load: one that can't be
// loaded leaves every calendar schedule with no answer to when it is next due.
func ValidateTimezone(timezone string) (string, error) {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return "", errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return "", fmt.Errorf("timezone %q is not a zone this service knows", timezone)
	}

	return timezone, nil
}

// ValidateWahoo checks the shared OAuth application and returns it trimmed.
// Every part of it may be empty: a service that has not been configured yet
// is a state this validation accepts and a run refuses.
func ValidateWahoo(wahoo Wahoo) (Wahoo, error) {
	wahoo.APIBaseURL = strings.TrimSpace(wahoo.APIBaseURL)
	wahoo.OAuthBaseURL = strings.TrimSpace(wahoo.OAuthBaseURL)
	wahoo.ClientID = strings.TrimSpace(wahoo.ClientID)

	if wahoo.APIBaseURL != "" {
		if err := ValidateHTTPSOrigin("wahoo.api_base_url", wahoo.APIBaseURL); err != nil {
			return Wahoo{}, err
		}
	}
	if wahoo.OAuthBaseURL != "" {
		if err := ValidateHTTPSOrigin("wahoo.oauth_base_url", wahoo.OAuthBaseURL); err != nil {
			return Wahoo{}, err
		}
	}

	return wahoo, nil
}

// ValidateSources checks the libraries a run reads and returns them trimmed. An
// empty list is a service with nothing to read yet rather than a mistake; a
// provider named twice is a mistake, because a run reads each provider once and
// stores its inventory under that provider's name.
func ValidateSources(raw []Source) ([]Source, error) {
	sources := make([]Source, 0, len(raw))
	for index, source := range raw {
		if _, _, known := SourceSecretNames(source.Provider); !known {
			return nil, fmt.Errorf("sources[%d].provider %q is not a known source", index, source.Provider)
		}
		if slices.ContainsFunc(sources, func(seen Source) bool { return seen.Provider == source.Provider }) {
			return nil, fmt.Errorf("sources[%d].provider is duplicated", index)
		}
		baseURL := strings.TrimSpace(source.BaseURL)
		name := fmt.Sprintf("sources[%d].base_url", index)
		if err := ValidateHTTPSOrigin(name, baseURL); err != nil {
			return nil, err
		}
		sources = append(sources, Source{Provider: source.Provider, BaseURL: baseURL})
	}
	if len(sources) == 0 {
		return nil, nil
	}

	return sources, nil
}

// ValidateSync checks the reconciliation settings. The deletion gate is a
// boolean and cannot be wrong; the staleness bound can.
func ValidateSync(sync Sync) error {
	if sync.StaleAfter < minimumInterval {
		return errors.New("sync.stale_after must be at least 1s")
	}
	if sync.InitialDelay < minimumInterval {
		return errors.New("sync.initial_delay must be at least 1s")
	}

	return nil
}

// ValidateNotifications checks the origin the credentials are sent to.
func ValidateNotifications(notifications Notifications) (Notifications, error) {
	notifications.PushoverBaseURL = strings.TrimSpace(notifications.PushoverBaseURL)
	if err := ValidateHTTPSOrigin("notifications.pushover.base_url", notifications.PushoverBaseURL); err != nil {
		return Notifications{}, err
	}

	return notifications, nil
}

// ValidateBasemaps checks the list the map may be switched between and returns it
// trimmed. At least one entry is required, and each is named: the name is both
// the label the reader picks by and the identity a browser remembers by, so a
// repeated name would make those disagree. The same-origin rule on a dark twin
// holds per entry rather than across the list.
func ValidateBasemaps(raw []Basemap) ([]Basemap, error) {
	if len(raw) == 0 {
		return nil, errors.New("webui.basemaps must contain at least one entry")
	}

	basemaps := make([]Basemap, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, entry := range raw {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, fmt.Errorf("webui.basemaps[%d].name is required", index)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("webui.basemaps[%d].name is duplicated", index)
		}
		seen[name] = struct{}{}

		styleURL := strings.TrimSpace(entry.StyleURL)
		if err := ValidateStyleURL(fmt.Sprintf("webui.basemaps[%d].style_url", index), styleURL); err != nil {
			return nil, err
		}

		styleURLDark := strings.TrimSpace(entry.StyleURLDark)
		if styleURLDark != "" {
			if entry.DarkCartography {
				return nil, fmt.Errorf(
					"webui.basemaps[%d] must not set both style_url_dark and dark_cartography", index)
			}
			darkName := fmt.Sprintf("webui.basemaps[%d].style_url_dark", index)
			if err := ValidateStyleURL(darkName, styleURLDark); err != nil {
				return nil, err
			}
			if !SameOrigin(styleURLDark, styleURL) {
				return nil, fmt.Errorf("%s must be on the same origin as webui.basemaps[%d].style_url", darkName, index)
			}
		}

		basemaps[index] = Basemap{
			Name:            name,
			StyleURL:        styleURL,
			StyleURLDark:    styleURLDark,
			DarkCartography: entry.DarkCartography,
		}
	}

	return basemaps, nil
}

// ValidateStyleURL accepts an absolute HTTPS style document URL. Unlike the
// service's own endpoints it permits a query string, because providers that
// require an API key carry it there; such a key is published to the browser and
// is the operator's deliberate choice, not a managed secret.
func ValidateStyleURL(name, value string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials or fragment", name)
	}

	return nil
}

// SameOrigin reports whether two URLs share a scheme and host. Hosts are
// compared case-insensitively because a host is not case-sensitive, and a
// difference in case would otherwise reject a pair the browser treats as one
// origin.
func SameOrigin(left, right string) bool {
	first, err := url.ParseRequestURI(left)
	if err != nil {
		return false
	}
	second, err := url.ParseRequestURI(right)
	if err != nil {
		return false
	}

	return first.Scheme == second.Scheme && strings.EqualFold(first.Host, second.Host)
}

// regionSlug is one Geofabrik region path, such as
// "europe/germany/rheinland-pfalz". A slug becomes a path under a fixed download
// host, so its shape is checked rather than trusted: lowercase segments of
// letters, digits and single hyphens introduce neither a host nor a traversal.
// The index builder applies the same rule before composing a URL.
var regionSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

// ValidateSurface accepts a set of regions to index, or no regions at all, and
// returns the list trimmed. An empty list is the operator's switch for leaving
// stages unclassified, so it is a valid setting rather than a missing one.
func ValidateSurface(surface Surface) (Surface, error) {
	regions := TrimmedRegions(surface.Regions)
	for _, region := range regions {
		if !regionSlug.MatchString(region) {
			return Surface{}, fmt.Errorf(
				"surface.regions entry %q must be a region path such as "+
					"\"europe/germany/rheinland-pfalz\"", region,
			)
		}
	}
	// Required whether or not a region is named: the rebuild schedule is created
	// either way, so a cadence of zero would be a schedule that could not start.
	if surface.RebuildInterval < minimumInterval {
		return Surface{}, errors.New("surface.rebuild_interval must be at least 1s")
	}

	surface.Regions = regions

	return surface, nil
}

// TrimmedRegions drops blank entries and repeats, keeping the order they were
// written in. A repeated region downloads the same extract and appends the same
// ways twice, which costs a build but cannot change what the index answers.
func TrimmedRegions(regions []string) []string {
	trimmed := make([]string, 0, len(regions))
	seen := make(map[string]bool, len(regions))
	for _, region := range regions {
		if region = strings.TrimSpace(region); region != "" && !seen[region] {
			seen[region] = true
			trimmed = append(trimmed, region)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}

	return trimmed
}

// ValidateHTTPSURL accepts an absolute HTTPS URL carrying nothing but a scheme,
// a host, and a path.
func ValidateHTTPSURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials, query, or fragment", name)
	}

	return nil
}

// ValidateHTTPSOrigin is ValidateHTTPSURL plus the absence of a path, which is
// what a setting the client parses as an origin has to be. Rejecting a path
// here rather than letting the client reject it keeps the failure where the
// operator can read it, naming the setting.
func ValidateHTTPSOrigin(name, value string) error {
	if err := ValidateHTTPSURL(name, value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("%s must be an origin, without a path", name)
	}

	return nil
}
