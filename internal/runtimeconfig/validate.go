package runtimeconfig

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// maxDigestInterval bounds the period one digest may cover.
//
// A digest totals the recorded run history, and that history is a bounded
// window: the store keeps the last few hundred runs, which an hourly deployment
// fills in a little over a week. A longer period would not fail — it would
// quietly report a total missing every run already pruned from under it, which
// is worse than refusing the setting.
const maxDigestInterval = 7 * 24 * time.Hour

// minimumInterval is the floor under every duration here. These settings cross
// the wire and are stored as whole seconds, so anything shorter is written down
// as zero and then refused when it is read back at startup.
const minimumInterval = time.Second

// Validate checks every rule and returns the normalised settings: lists trimmed
// of whitespace and repeats, so what is stored is exactly what was checked.
//
//nolint:gocritic // value receiver: validation returns a normalised copy rather than editing the caller's.
func (v Values) Validate() (Values, error) {
	if err := ValidateSync(v.Sync); err != nil {
		return Values{}, err
	}
	if err := ValidateNotifications(v.Notifications); err != nil {
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

	v.Basemaps = basemaps
	v.Surface = surface

	return v, nil
}

// ValidateSync checks the reconciliation settings. The deletion gate is a
// boolean and cannot be wrong; the staleness bound can.
func ValidateSync(sync Sync) error {
	if sync.StaleAfter < minimumInterval {
		return errors.New("sync.stale_after must be at least 1s")
	}

	return nil
}

// ValidateNotifications checks the success policy, the period a digest covers,
// and the origin the credentials are sent to. A digest with no positive
// interval would either never be sent or be sent on every run, and neither is
// what the setting means.
func ValidateNotifications(notifications Notifications) error {
	switch notifications.Policy {
	case SuccessPolicyEvery, SuccessPolicyQuiet, SuccessPolicyDigest:
	default:
		return errors.New("notifications.success_policy must be every, quiet, or digest")
	}
	// The period is checked whatever the policy reads it. A setting that is only
	// consulted by one policy is still a setting an operator will switch to, and
	// finding out then that it was never valid is the wrong moment.
	if notifications.DigestInterval < minimumInterval {
		return errors.New("notifications.digest_interval must be at least 1s")
	}
	if notifications.DigestInterval > maxDigestInterval {
		return fmt.Errorf(
			"notifications.digest_interval must not exceed %s, which is as far back as the recorded run history reaches",
			maxDigestInterval,
		)
	}

	return ValidateHTTPSOrigin("notifications.pushover.base_url", notifications.PushoverBaseURL)
}

// ValidateBasemaps checks the list the map may be switched between and returns
// it trimmed. At least one entry is required, because a map with no cartography
// paints nothing; each is named, because the name is both the label the reader
// picks by and the identity a browser remembers its choice by, and a repeated
// name would make those two disagree.
//
// The same-origin rule on a dark twin holds per entry rather than across the
// list. One entry still reaches one third-party origin, and the page still
// requests only the entry currently on screen; what the list grows is the set of
// providers the operator has offered, which is the point of the setting.
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
// "europe/germany/rheinland-pfalz".
//
// A slug becomes a path under a fixed download host, so its shape is checked
// here rather than trusted: lowercase segments of letters, digits, and single
// hyphens can introduce neither a host nor a traversal. The index builder
// applies the same rule again before it composes a URL — this one exists so a
// mistyped region is refused where it is entered, rather than becoming a
// download that fails a week later.
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
	// Required whether or not a region is named. The rebuild schedule is
	// created either way — a service with no regions runs it and does nothing —
	// so a cadence of zero would be a schedule that could not be started, and an
	// operator naming their first region would find out only at the next
	// restart.
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
