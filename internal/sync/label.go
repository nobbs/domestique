package sync

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
)

// DeviceRoute is one route as the Wahoo device API lists it.
type DeviceRoute struct {
	ExternalID string
	ProviderID string
	ID         int64
}

// DeviceAPI is the Wahoo API an ELEMNT signs in to, the only one that can
// write a route's provider_id.
type DeviceAPI interface {
	SignIn(ctx context.Context, email, password []byte) (token string, err error)
	Routes(ctx context.Context, token string) ([]DeviceRoute, error)
	SetProviderID(ctx context.Context, token string, routeID int64, value string) error
	IsUnauthorized(err error) bool
}

// LabelState is the durable state labelling reads and writes: whose target
// it is, and that rider's own Wahoo credentials and device session.
type LabelState interface {
	TargetOwner(ctx context.Context, targetID string) (string, error)
	RiderCredentials(ctx context.Context, subject string) (map[rider.CredentialName]rider.Credential, error)
	SetRiderCredentials(ctx context.Context, subject string, credentials map[rider.CredentialName]rider.Credential) error
}

// DeviceLabeler gives every route this service owns on a target the
// provider_id an ELEMNT keys it by, which the public API leaves empty so the
// device would otherwise keep only one of them.
type DeviceLabeler struct {
	api   DeviceAPI
	state LabelState
}

// NewDeviceLabeler builds a labeler over the device API and the store.
func NewDeviceLabeler(api DeviceAPI, state LabelState) (*DeviceLabeler, error) {
	if api == nil || state == nil {
		return nil, errors.New("sync: a device API and a state are required")
	}

	return &DeviceLabeler{api: api, state: state}, nil
}

// Label fills the empty provider_id of every owned route on one target with
// that route's own id, and never changes one already set: a device keeps the
// entry an earlier value made. It never fails a run; a rider without Wahoo
// credentials, or whose last sign-in was refused, is skipped until they save
// new ones, and anything else is logged as counts and a reason.
func (l *DeviceLabeler) Label(ctx context.Context, targetID string) {
	subject, err := l.state.TargetOwner(ctx, targetID)
	if err != nil {
		logLabel(0, 0, "state")

		return
	}
	if subject == "" {
		return
	}
	credentials, err := l.state.RiderCredentials(ctx, subject)
	if err != nil {
		logLabel(0, 0, "state")

		return
	}
	email, password := credentials[rider.CredentialWahooEmail], credentials[rider.CredentialWahooPassword]
	if !email.IsSet() || !password.IsSet() || credentials[rider.CredentialWahooRefused].IsSet() {
		return
	}

	session := string(credentials[rider.CredentialWahooSession].Bytes())
	token, routes, reason := l.signedInRoutes(ctx, subject, email.Bytes(), password.Bytes(), session)
	if reason != "" {
		logLabel(0, 0, reason)

		return
	}

	labelled, failed := 0, 0
	for _, owned := range routes {
		if !route.OwnsExternalID(owned.ExternalID) || owned.ProviderID != "" {
			continue
		}
		if err := l.api.SetProviderID(ctx, token, owned.ID, strconv.FormatInt(owned.ID, 10)); err != nil {
			failed++
			if l.api.IsUnauthorized(err) {
				logLabel(labelled, failed, "session")

				return
			}

			continue
		}
		labelled++
	}
	switch {
	case failed > 0:
		logLabel(labelled, failed, "write")
	case labelled > 0:
		logLabel(labelled, 0, "")
	}
}

// signedInRoutes lists the rider's routes with the stored session, signing in
// afresh only when there is none or Wahoo no longer accepts it. A new session
// is stored; refused credentials are marked for the settings page.
func (l *DeviceLabeler) signedInRoutes(
	ctx context.Context, subject string, email, password []byte, token string,
) (session string, routes []DeviceRoute, reason string) {
	if token != "" {
		listed, err := l.api.Routes(ctx, token)
		if err == nil {
			return token, listed, ""
		}
		if !l.api.IsUnauthorized(err) {
			return "", nil, "listing"
		}
	}

	token, err := l.api.SignIn(ctx, email, password)
	if err != nil {
		if !l.api.IsUnauthorized(err) {
			return "", nil, "sign_in"
		}
		if storeErr := l.state.SetRiderCredentials(ctx, subject, map[rider.CredentialName]rider.Credential{
			rider.CredentialWahooSession: {},
			rider.CredentialWahooRefused: rider.NewCredential([]byte("1")),
		}); storeErr != nil {
			return "", nil, "state"
		}

		return "", nil, "refused"
	}
	if storeErr := l.state.SetRiderCredentials(ctx, subject, map[rider.CredentialName]rider.Credential{
		rider.CredentialWahooSession: rider.NewCredential([]byte(token)),
		rider.CredentialWahooRefused: {},
	}); storeErr != nil {
		return "", nil, "state"
	}

	listed, err := l.api.Routes(ctx, token)
	if err != nil {
		return "", nil, "listing"
	}

	return token, listed, ""
}

// logLabel is the one place labelling is heard in the log: counts and a
// stable reason, never a route, an email or a token.
func logLabel(labelled, failed int, reason string) {
	if reason == "" {
		slog.Info("device route labels written", "labelled", labelled, "failed", failed)

		return
	}
	slog.Warn("device route labelling incomplete", "labelled", labelled, "failed", failed, "reason", reason)
}
