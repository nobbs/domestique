package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// basemapsPath is the one route whose body is allowed to be larger than every
// other; see requestLimit.
const basemapsPath = "/v1/settings/basemaps"

// styleRefreshBudget bounds how long a basemap save waits for the saved styles
// to be read. Short enough that an unreachable provider costs the operator a
// pause rather than a hung form.
const styleRefreshBudget = 5 * time.Second

// GetSettings serves the settings that are in force right now, which is what
// the form the operator edits is filled from.
func (h *Handler) GetSettings(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, h.settingsView())
}

// The settings are written one section at a time, and each of these replaces the
// whole of the section it names. The page is a form per section, so a save
// carries what its own form holds and touches nothing else. Credentials travel
// with their section and only the ones typed in: one left out keeps its stored
// value, one sent empty is removed.

// SetWahooApplication replaces the registered application every self-service
// target authorizes against. Which targets exist is not part of this
// section: each rider creates their own by connecting their own account.
func (h *Handler) SetWahooApplication(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.WahooApplicationUpdate](h, writer, request)
	if !ok {
		return
	}
	h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Wahoo.APIBaseURL = body.APIBaseURL
		values.Wahoo.OAuthBaseURL = body.OauthBaseURL
		values.Wahoo.ClientID = body.ClientID

		return values
	}, submitted(map[runtimeconfig.SecretName]*string{
		runtimeconfig.SecretWahooClientSecret: body.ClientSecret,
	}))
}

// SetSource replaces one library and the account it is read with, leaving the
// other libraries as they are.
func (h *Handler) SetSource(writer http.ResponseWriter, request *http.Request) {
	// The contract names the libraries this path accepts, and a request for any
	// other is refused before it reaches here.
	provider := route.Provider(request.PathValue("provider"))
	email, password, _ := runtimeconfig.SourceSecretNames(provider)
	body, ok := settingsBody[openapi.SourceUpdate](h, writer, request)
	if !ok {
		return
	}
	h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		// Rebuilt from the list of every library rather than spliced, so the
		// libraries stay in the order a run reads them however they are turned
		// on and off.
		read := make([]runtimeconfig.Source, 0, len(runtimeconfig.SourceProviders()))
		for _, each := range runtimeconfig.SourceProviders() {
			switch {
			case each == provider && body.Read:
				read = append(read, runtimeconfig.Source{Provider: provider, BaseURL: body.BaseURL})
			case each == provider:
			default:
				if stored := sourceOf(values.Sources, each); stored != nil {
					read = append(read, *stored)
				}
			}
		}
		values.Sources = read

		return values
	}, submitted(map[runtimeconfig.SecretName]*string{email: body.Email, password: body.Password}))
}

// SetNotifications replaces what reaches the operator's phone, and where it is
// sent.
func (h *Handler) SetNotifications(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.NotificationsUpdate](h, writer, request)
	if !ok {
		return
	}
	h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Notifications = runtimeconfig.Notifications{
			Enabled:         body.Enabled,
			PushoverBaseURL: body.PushoverBaseURL,
		}

		return values
	}, submitted(map[runtimeconfig.SecretName]*string{
		runtimeconfig.SecretPushoverApplicationToken: body.ApplicationToken,
		runtimeconfig.SecretPushoverUserKey:          body.UserKey,
	}))
}

// SetBasemaps replaces the cartographies the reader may switch the map between.
func (h *Handler) SetBasemaps(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.BasemapsUpdate](h, writer, request)
	if !ok {
		return
	}
	stored := h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Basemaps = make([]runtimeconfig.Basemap, len(body.Basemaps))
		for index, basemap := range body.Basemaps {
			values.Basemaps[index] = runtimeconfig.Basemap{
				Name:            basemap.Name,
				StyleURL:        basemap.StyleURL,
				StyleURLDark:    stringValue(basemap.StyleURLDark),
				DarkCartography: boolValue(basemap.DarkCartography),
			}
		}

		return values
	}, nil)
	if stored && h.styleOrigins != nil {
		// The saved styles are read now rather than at the next scheduled read, so
		// the policy on the responses after this save already admits whatever
		// hosts they name. Bounded, because a provider that has gone away must not
		// hold the save open: what does not resolve inside the budget is picked up
		// by the scheduled read instead.
		ctx, cancel := context.WithTimeout(request.Context(), styleRefreshBudget)
		defer cancel()
		h.styleOrigins.Refresh(ctx)
	}
}

// SetSurface replaces the regions the surface index is built from and how often
// it is rebuilt. A stored edit starts the rebuild the new regions or cadence
// feed.
func (h *Handler) SetSurface(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.SurfaceSettings](h, writer, request)
	if !ok {
		return
	}
	stored := h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Surface = runtimeconfig.Surface{
			Regions:         slices.Clone(body.Regions),
			RebuildInterval: time.Duration(body.RebuildIntervalSeconds) * time.Second,
		}

		return values
	}, nil)
	if stored {
		// The trigger result is ignored: a refused start means the work is
		// already happening.
		h.tasks.Run(TaskSurfaceIndex, "")
	}
}

// SetRideModel replaces the coefficient file predicted moving time is computed
// from. A stored edit starts the prediction pass that consumes it.
func (h *Handler) SetRideModel(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.RideModelSettings](h, writer, request)
	if !ok {
		return
	}
	stored := h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.RideModel = runtimeconfig.RideModel{CoefficientsFile: body.CoefficientsFile}

		return values
	}, nil)
	if stored {
		h.tasks.Run(TaskRideModelPredict, "")
	}
}

// SetSync replaces the settings a run reads when it starts.
func (h *Handler) SetSync(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.SyncSettings](h, writer, request)
	if !ok {
		return
	}
	h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Sync = runtimeconfig.Sync{
			AllowEmptySourceDeletion: body.AllowEmptySourceDeletion,
			StaleAfter:               time.Duration(body.StaleAfterSeconds) * time.Second,
			InitialDelay:             time.Duration(body.InitialDelaySeconds) * time.Second,
		}

		return values
	}, nil)
}

// settingsBody reads one section's submitted edit.
func settingsBody[Body any](h *Handler, writer http.ResponseWriter, request *http.Request) (Body, bool) {
	var body Body
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body is not this section")

		return body, false
	}
	// One object, and nothing after it, for the reason SetSyncSchedule gives:
	// acting on the first half of a body the caller believes was read whole is
	// how a setting ends up in a state nobody asked for.
	if decoder.More() {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body must be one object")

		return body, false
	}

	return body, true
}

// storeSection commits one section and its credentials together, applied to the
// settings as they are at the moment of the write, and answers with every setting now in force.
// It reports true only when the 200 settings view was written, so a caller can
// start work the section it just wrote feeds.
func (h *Handler) storeSection(
	writer http.ResponseWriter,
	request *http.Request,
	change func(runtimeconfig.Values) runtimeconfig.Values,
	secrets map[runtimeconfig.SecretName]runtimeconfig.Secret,
) bool {
	// The rules are the runtime settings package's own — the same ones the
	// stored values were read back through at startup — so the message names
	// the setting that is wrong rather than the section it was in.
	if err := h.settings.UpdateWithSecrets(request.Context(), change, secrets); err != nil {
		if errors.Is(err, runtimeconfig.ErrStore) {
			h.unavailable(writer)

			return false
		}
		h.error(writer, http.StatusBadRequest, "invalid_request", err.Error())

		return false
	}
	h.writeJSON(writer, http.StatusOK, h.settingsView())

	return true
}

// declaredAlert identifies one alert in the matrix. It is what an alert is,
// rather than what was decided about it, so a lookup cannot turn on a decision.
type declaredAlert struct {
	task  string
	alert string
}

// SetTimezone replaces the zone this service reads local time in.
func (h *Handler) SetTimezone(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.TimezoneUpdate](h, writer, request)
	if !ok {
		return
	}
	h.storeSection(writer, request, func(values runtimeconfig.Values) runtimeconfig.Values {
		values.Timezone = body.Timezone

		return values
	}, nil)
}

// SetAlerts records which alerts an operator wants delivered. An alert left out
// of the request keeps whatever it had: deciding is what creates a record, and
// an absent decision is not the same as switching one off.
func (h *Handler) SetAlerts(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.AlertsUpdate](h, writer, request)
	if !ok {
		return
	}

	catalogue := h.alerts.Catalogue()
	known := make(map[declaredAlert]struct{}, len(catalogue))
	for _, alert := range catalogue {
		known[declaredAlert{task: alert.Task, alert: alert.Alert}] = struct{}{}
	}

	decisions := make([]AlertDecision, 0, len(body.Alerts))
	for _, decision := range body.Alerts {
		// Storing a decision about an alert nothing raises would leave a row
		// nobody ever reads and a switch the page shows as having an effect.
		if _, declared := known[declaredAlert{task: decision.Task, alert: decision.Alert}]; !declared {
			// The reason is named without the request's own strings in it: an
			// error page is observable, and the page only ever sends alerts
			// from the matrix it was served.
			h.error(writer, http.StatusBadRequest, "invalid_request",
				"the request names an alert no task raises")

			return
		}
		decisions = append(decisions, AlertDecision{
			Task:    decision.Task,
			Alert:   decision.Alert,
			Enabled: decision.Enabled,
		})
	}
	if err := h.alerts.Decide(request.Context(), decisions); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, h.settingsView())
}

// alertSettings renders the alert matrix. It is always a list, never null: an
// operator who has decided nothing yet still has a matrix to read.
func alertSettings(catalogue []AlertSetting) []openapi.AlertSetting {
	settings := make([]openapi.AlertSetting, 0, len(catalogue))
	for _, alert := range catalogue {
		settings = append(settings, openapi.AlertSetting{
			Task:    alert.Task,
			Alert:   alert.Alert,
			Enabled: alert.Enabled,
			Decided: alert.Decided,
		})
	}

	return settings
}

// submitted collects the credentials an edit actually carried. One left out is
// absent here rather than empty, because absent keeps what is stored and empty
// removes it.
func submitted(values map[runtimeconfig.SecretName]*string) map[runtimeconfig.SecretName]runtimeconfig.Secret {
	secrets := make(map[runtimeconfig.SecretName]runtimeconfig.Secret, len(values))
	for name, value := range values {
		if value != nil {
			secrets[name] = runtimeconfig.NewSecret([]byte(*value))
		}
	}

	return secrets
}

// sourceOf finds one library in the list a run reads, or nil if it is off.
func sourceOf(sources []runtimeconfig.Source, provider route.Provider) *runtimeconfig.Source {
	for index, source := range sources {
		if source.Provider == provider {
			return &sources[index]
		}
	}

	return nil
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
			Enabled:         values.Notifications.Enabled,
			PushoverBaseURL: values.Notifications.PushoverBaseURL,
		},
		Wahoo: openapi.WahooSettings{
			APIBaseURL:   values.Wahoo.APIBaseURL,
			OauthBaseURL: values.Wahoo.OAuthBaseURL,
			ClientID:     values.Wahoo.ClientID,
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
		Timezone:   values.Timezone,
		Alerts:     alertSettings(h.alerts.Catalogue()),
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
