package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// settingsPath is the one route whose body is allowed to be larger than every
// other; see requestLimit.
const settingsPath = "/v1/settings"

// GetSettings serves the settings that are in force right now, which is what
// the form the operator edits is filled from.
func (h *Handler) GetSettings(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, h.settingsView())
}

// SetSettings replaces every runtime setting at once.
//
// The body is the whole object rather than the fields that changed. The form
// that sends it holds every value, and a merge would let a page that had gone
// stale reinstate a setting its reader never looked at. Each section is
// therefore required, and the response echoes what is now stored, normalised —
// so the page's copy is the service's copy without a second read.
//
// Credentials are the exception: only the ones sent are written, because the
// page is never told what the others are and so has nothing to send back.
func (h *Handler) SetSettings(writer http.ResponseWriter, request *http.Request) {
	var body settingsBody
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumSettingsBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !body.complete() {
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
	submitted, err := body.values().Validate()
	if err != nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", err.Error())

		return
	}
	secrets, err := body.secrets()
	if err != nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", err.Error())

		return
	}

	if err := h.settings.SetSecrets(request.Context(), secrets); err != nil {
		h.unavailable(writer)

		return
	}
	if err := h.settings.Set(request.Context(), submitted); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, h.settingsView())
}

// settingsBody is one submitted edit. Every section is a pointer so that a body
// which left one out is refused rather than quietly storing that section's zero
// values.
type settingsBody struct {
	Sync          *openapi.SyncSettings         `json:"sync"`
	Notifications *openapi.NotificationSettings `json:"notifications"`
	Basemaps      *[]openapi.BrowserBasemap     `json:"basemaps"`
	Surface       *openapi.SurfaceSettings      `json:"surface"`
	Wahoo         *openapi.WahooSettings        `json:"wahoo"`
	Sources       *[]openapi.SourceSettings     `json:"sources"`
	RideModel     *openapi.RideModelSettings    `json:"rideModel"`
	Secrets       map[string]string             `json:"secrets"`
}

// complete reports whether every section the service replaces whole is present.
func (b *settingsBody) complete() bool {
	return b.Sync != nil && b.Notifications != nil && b.Basemaps != nil && b.Surface != nil &&
		b.Wahoo != nil && b.Sources != nil && b.RideModel != nil
}

// values reads one submitted body into the settings the service holds.
// Durations cross the wire as whole seconds, the unit the status response
// already reports an age in.
func (b *settingsBody) values() runtimeconfig.Values {
	values := runtimeconfig.Values{
		Sync: runtimeconfig.Sync{
			AllowEmptySourceDeletion: b.Sync.AllowEmptySourceDeletion,
			StaleAfter:               time.Duration(b.Sync.StaleAfterSeconds) * time.Second,
			InitialDelay:             time.Duration(b.Sync.InitialDelaySeconds) * time.Second,
		},
		Notifications: runtimeconfig.Notifications{
			Enabled:         b.Notifications.Enabled,
			Policy:          runtimeconfig.SuccessPolicy(b.Notifications.SuccessPolicy),
			DigestInterval:  time.Duration(b.Notifications.DigestIntervalSeconds) * time.Second,
			PushoverBaseURL: b.Notifications.PushoverBaseURL,
		},
		Wahoo: runtimeconfig.Wahoo{
			APIBaseURL:   b.Wahoo.APIBaseURL,
			OAuthBaseURL: b.Wahoo.OauthBaseURL,
			ClientID:     b.Wahoo.ClientID,
			Targets:      append([]string(nil), b.Wahoo.Targets...),
		},
		RideModel: runtimeconfig.RideModel{CoefficientsFile: b.RideModel.CoefficientsFile},
		Basemaps:  make([]runtimeconfig.Basemap, len(*b.Basemaps)),
		Sources:   make([]runtimeconfig.Source, len(*b.Sources)),
		Surface: runtimeconfig.Surface{
			Regions:         append([]string(nil), b.Surface.Regions...),
			RebuildInterval: time.Duration(b.Surface.RebuildIntervalSeconds) * time.Second,
		},
	}
	for index, basemap := range *b.Basemaps {
		values.Basemaps[index] = runtimeconfig.Basemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    stringValue(basemap.StyleURLDark),
			DarkCartography: boolValue(basemap.DarkCartography),
		}
	}
	for index, source := range *b.Sources {
		values.Sources[index] = runtimeconfig.Source{
			Provider: route.Provider(source.Provider),
			BaseURL:  source.BaseURL,
		}
	}

	return values
}

// secrets reads the credentials this edit replaces. A name nothing reads is
// refused here rather than stored somewhere nothing would look for it, and an
// empty value is the credential being removed.
func (b *settingsBody) secrets() (map[runtimeconfig.SecretName]runtimeconfig.Secret, error) {
	secrets := make(map[runtimeconfig.SecretName]runtimeconfig.Secret, len(b.Secrets))
	for submitted, value := range b.Secrets {
		name, err := runtimeconfig.ParseSecretName(submitted)
		if err != nil {
			return nil, fmt.Errorf("reading the credentials to replace: %w", err)
		}
		secrets[name] = runtimeconfig.NewSecret([]byte(value))
	}

	return secrets, nil
}

// settingsView is the wire form of the settings in force. The credentials are
// in it only as whether they are set: this handler is never told their values,
// so there is nothing here for a response to leak.
func (h *Handler) settingsView() openapi.Settings {
	values := h.settings.Values()
	view := openapi.Settings{
		Sync: openapi.SyncSettings{
			AllowEmptySourceDeletion: values.Sync.AllowEmptySourceDeletion,
			StaleAfterSeconds:        int(values.Sync.StaleAfter / time.Second),
			InitialDelaySeconds:      int(values.Sync.InitialDelay / time.Second),
		},
		Notifications: openapi.NotificationSettings{
			Enabled:               values.Notifications.Enabled,
			SuccessPolicy:         openapi.NotificationSettings_SuccessPolicy(values.Notifications.Policy),
			DigestIntervalSeconds: int(values.Notifications.DigestInterval / time.Second),
			PushoverBaseURL:       values.Notifications.PushoverBaseURL,
		},
		Wahoo: openapi.WahooSettings{
			APIBaseURL:   values.Wahoo.APIBaseURL,
			OauthBaseURL: values.Wahoo.OAuthBaseURL,
			ClientID:     values.Wahoo.ClientID,
			Targets:      append([]string{}, values.Wahoo.Targets...),
		},
		RideModel: openapi.RideModelSettings{CoefficientsFile: values.RideModel.CoefficientsFile},
		Basemaps:  make([]openapi.BrowserBasemap, len(values.Basemaps)),
		// An empty list is sent as one rather than as null: no source, no target
		// and no region are all states the service runs in, and the page has to
		// be able to tell each from a field it was not given.
		Sources: make([]openapi.SourceSettings, 0, len(values.Sources)),
		Surface: openapi.SurfaceSettings{
			Regions:                append([]string{}, values.Surface.Regions...),
			RebuildIntervalSeconds: int(values.Surface.RebuildInterval / time.Second),
		},
		SecretsSet: make(map[string]bool, len(runtimeconfig.SecretNames())),
		Missing:    append([]string{}, h.settings.Missing()...),
	}
	for index, basemap := range values.Basemaps {
		view.Basemaps[index] = openapi.BrowserBasemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    optionalString(basemap.StyleURLDark),
			DarkCartography: optionalBool(basemap.DarkCartography),
		}
	}
	for _, source := range values.Sources {
		view.Sources = append(view.Sources, openapi.SourceSettings{
			Provider: openapi.SourceSettings_Provider(source.Provider),
			BaseURL:  source.BaseURL,
		})
	}
	for _, name := range runtimeconfig.SecretNames() {
		view.SecretsSet[string(name)] = h.settings.SecretIsSet(name)
	}

	return view
}
