package httpapi

import (
	"context"
	"fmt"
	"net/http"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/rider"
)

// The rider's own profile, which is the one settings section that is not the
// service's. It is not admin-gated and not admin-readable either: every read
// and write here is over the caller's own subject, so an administrator reads
// their own profile from this path and nobody else's.

// GetRiderProfile serves the caller's own parameters and what their recent
// rides suggest.
func (h *Handler) GetRiderProfile(writer http.ResponseWriter, request *http.Request) {
	h.writeRiderProfile(writer, request)
}

// SetRiderProfile replaces the caller's own parameters whole.
func (h *Handler) SetRiderProfile(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.RiderParameters](h, writer, request)
	if !ok {
		return
	}
	profile := rider.Profile{
		MaxHeartRateBPM:               rider.FromPointer(body.MaxHeartRateBpm),
		RestingHeartRateBPM:           rider.FromPointer(body.RestingHeartRateBpm),
		ThresholdHeartRateBPM:         rider.FromPointer(body.ThresholdHeartRateBpm),
		FunctionalThresholdPowerWatts: rider.FromPointer(body.FunctionalThresholdPowerWatts),
		RiderMassKG:                   rider.FromPointer(body.RiderMassKg),
		BikeMassKG:                    rider.FromPointer(body.BikeMassKg),
	}
	ctx := request.Context()
	if err := h.state.SetRiderProfile(ctx, identityOf(ctx).Subject, profile); err != nil {
		h.unavailable(writer)

		return
	}
	// Everything derived from these numbers is now worked out against values
	// nobody holds any more, so the derivation is started over this rider's own
	// targets. A refused start means that work is already happening, and the
	// task recomputes against the profile as it stands when it runs.
	if targetIDs, err := h.ownTargetIDs(ctx); err == nil {
		for _, targetID := range targetIDs {
			h.tasks.Run(TaskActivityDerive, targetID)
		}
	}
	h.writeRiderProfile(writer, request)
}

// writeRiderProfile answers a read and a write alike: what the rider now holds,
// beside what their rides suggest.
func (h *Handler) writeRiderProfile(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	profile, err := h.state.RiderProfile(ctx, identityOf(ctx).Subject)
	if err != nil {
		h.unavailable(writer)

		return
	}
	// The suggestions are read over the caller's own targets, which for an
	// admin is still only their own: this path is never another rider's.
	targetIDs, err := h.ownTargetIDs(ctx)
	if err != nil {
		h.unavailable(writer)

		return
	}
	suggestions, err := h.state.RiderSuggestions(
		ctx, targetIDs, h.stoppingTypes, h.now().Add(-rider.SuggestionWindow),
	)
	if err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.RiderProfile{
		Profile: openapi.RiderParameters{
			MaxHeartRateBpm:               profile.MaxHeartRateBPM.Pointer(),
			RestingHeartRateBpm:           profile.RestingHeartRateBPM.Pointer(),
			ThresholdHeartRateBpm:         profile.ThresholdHeartRateBPM.Pointer(),
			FunctionalThresholdPowerWatts: profile.FunctionalThresholdPowerWatts.Pointer(),
			RiderMassKg:                   profile.RiderMassKG.Pointer(),
			BikeMassKg:                    profile.BikeMassKG.Pointer(),
		},
		Suggestions: openapi.RiderSuggestions{
			MaxHeartRateBpm:               suggestions.MaxHeartRateBPM.Pointer(),
			FunctionalThresholdPowerWatts: suggestions.FunctionalThresholdPowerWatts.Pointer(),
			Stopping:                      stoppingSuggestion(suggestions.Stopping),
		},
	})
}

// ownTargetIDs are the caller's own targets, an admin's included. targetIDs
// widens an admin to every target, which is right where the answer is about the
// service and wrong where it is about one rider's own body.
func (h *Handler) ownTargetIDs(ctx context.Context) ([]string, error) {
	subject := identityOf(ctx).Subject
	ids := []string{}
	if err := h.state.ForEachTarget(ctx, func(id, _, ownerSubject string) error {
		if ownerSubject == subject {
			ids = append(ids, id)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing targets: %w", err)
	}

	return ids, nil
}

// stoppingSuggestion renders the measured habit the way an optional object is
// rendered: absent, rather than a set of zeroes, until enough rides carry it.
func stoppingSuggestion(stopping rider.Stopping) *openapi.StoppingSuggestion {
	if !stopping.Set {
		return nil
	}

	return &openapi.StoppingSuggestion{
		MedianSecondsPerHour:        stopping.MedianSecondsPerHour,
		LowerQuartileSecondsPerHour: stopping.LowerQuartileSecondsPerHour,
		UpperQuartileSecondsPerHour: stopping.UpperQuartileSecondsPerHour,
		Rides:                       stopping.Rides,
	}
}
