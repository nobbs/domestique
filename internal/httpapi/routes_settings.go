package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// settingsPath is the one route whose body is allowed to be larger than every
// other; see requestLimit.
const settingsPath = "/v1/settings"

// GetSettings serves the settings that are in force right now, which is what
// the form the operator edits is filled from.
func (h *Handler) GetSettings(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, settingsView(h.settings.Values()))
}

// SetSettings replaces every runtime setting at once.
//
// The body is the whole object rather than the fields that changed. The form
// that sends it holds every value, and a merge would let a page that had gone
// stale reinstate a setting its reader never looked at. Each of the four
// sections is therefore required, and the response echoes what is now stored,
// normalised — so the page's copy is the service's copy without a second read.
func (h *Handler) SetSettings(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Sync          *openapi.SyncSettings         `json:"sync"`
		Notifications *openapi.NotificationSettings `json:"notifications"`
		Basemaps      *[]openapi.BrowserBasemap     `json:"basemaps"`
		Surface       *openapi.SurfaceSettings      `json:"surface"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumSettingsBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil ||
		body.Sync == nil || body.Notifications == nil || body.Basemaps == nil || body.Surface == nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", "every settings section is required")

		return
	}
	// One object, and nothing after it, for the reason SetSyncSchedule gives:
	// acting on the first half of a body the caller believes was read whole is
	// how a setting ends up in a state nobody asked for.
	if decoder.More() {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body must be one object")

		return
	}

	// Checked before it is written, so the message names the setting that is
	// wrong. These rules are the runtime settings package's own — the same ones
	// the stored values were read back through at startup.
	submitted, err := settingsValues(body.Sync, body.Notifications, *body.Basemaps, body.Surface).Validate()
	if err != nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", err.Error())

		return
	}
	if err := h.settings.Set(request.Context(), submitted); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, settingsView(h.settings.Values()))
}

// settingsValues reads one submitted body into the settings the service holds.
// Durations cross the wire as whole seconds, the unit the status response
// already reports an age in.
func settingsValues(
	sync *openapi.SyncSettings,
	notifications *openapi.NotificationSettings,
	basemaps []openapi.BrowserBasemap,
	surface *openapi.SurfaceSettings,
) runtimeconfig.Values {
	values := runtimeconfig.Values{
		Sync: runtimeconfig.Sync{
			AllowEmptySourceDeletion: sync.AllowEmptySourceDeletion,
			StaleAfter:               time.Duration(sync.StaleAfterSeconds) * time.Second,
		},
		Notifications: runtimeconfig.Notifications{
			Enabled:         notifications.Enabled,
			Policy:          runtimeconfig.SuccessPolicy(notifications.SuccessPolicy),
			DigestInterval:  time.Duration(notifications.DigestIntervalSeconds) * time.Second,
			PushoverBaseURL: notifications.PushoverBaseURL,
		},
		Basemaps: make([]runtimeconfig.Basemap, len(basemaps)),
		Surface: runtimeconfig.Surface{
			Regions:         append([]string(nil), surface.Regions...),
			RebuildInterval: time.Duration(surface.RebuildIntervalSeconds) * time.Second,
		},
	}
	for index, basemap := range basemaps {
		values.Basemaps[index] = runtimeconfig.Basemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    stringValue(basemap.StyleURLDark),
			DarkCartography: boolValue(basemap.DarkCartography),
		}
	}

	return values
}

// settingsView is the wire form of the settings in force.
//
//nolint:gocritic // value param: the view is rendered from a snapshot copy.
func settingsView(values runtimeconfig.Values) openapi.Settings {
	view := openapi.Settings{
		Sync: openapi.SyncSettings{
			AllowEmptySourceDeletion: values.Sync.AllowEmptySourceDeletion,
			StaleAfterSeconds:        int(values.Sync.StaleAfter / time.Second),
		},
		Notifications: openapi.NotificationSettings{
			Enabled:               values.Notifications.Enabled,
			SuccessPolicy:         openapi.NotificationSettings_SuccessPolicy(values.Notifications.Policy),
			DigestIntervalSeconds: int(values.Notifications.DigestInterval / time.Second),
			PushoverBaseURL:       values.Notifications.PushoverBaseURL,
		},
		Basemaps: make([]openapi.BrowserBasemap, len(values.Basemaps)),
		// An empty list is sent as one rather than as null: no region is the
		// state that switches classification off, and the page has to be able to
		// tell it from a field it was not given.
		Surface: openapi.SurfaceSettings{
			Regions:                append([]string{}, values.Surface.Regions...),
			RebuildIntervalSeconds: int(values.Surface.RebuildInterval / time.Second),
		},
	}
	for index, basemap := range values.Basemaps {
		view.Basemaps[index] = openapi.BrowserBasemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    optionalString(basemap.StyleURLDark),
			DarkCartography: optionalBool(basemap.DarkCartography),
		}
	}

	return view
}
