package main

import (
	"testing"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/demo"
)

func demoTeam(t *testing.T) *team {
	t.Helper()

	local, err := newTeam(&config.CloudflareAccess{
		TeamDomain:     "demo",
		ApplicationAUD: "demo-application",
		AllowedEmail:   "rider@example.test",
	})
	if err != nil {
		t.Fatalf("newTeam() error = %v", err)
	}

	return local
}

// The demo is only usable if the assertion it hands the dev server passes the
// production gate, so that round trip is the thing worth asserting: a signature
// the real verifier accepts, over the identity the configuration allows.
func TestMintedAssertionPassesTheProductionGate(t *testing.T) {
	local := demoTeam(t)

	assertion, err := local.mint()
	if err != nil {
		t.Fatalf("mint() error = %v", err)
	}
	email, err := local.Verify(t.Context(), assertion)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if want := "rider@example.test"; email != want {
		t.Errorf("Verify() = %q, want %q", email, want)
	}
}

// A demo that could be talked into fetching keys from a real team would be a
// demo that reaches a provider, which is the one thing it must not do.
func TestKeySetTransportServesOnlyTheLocalTeam(t *testing.T) {
	local := demoTeam(t)

	if _, err := local.Verify(t.Context(), "not.a.token"); err == nil {
		t.Fatal("expected a malformed assertion to be rejected")
	}

	other := demoTeam(t)
	other.issuer = "https://elsewhere.example.test"
	assertion, err := local.mint()
	if err != nil {
		t.Fatalf("mint() error = %v", err)
	}
	if _, err := other.Verify(t.Context(), assertion); err == nil {
		t.Fatal("expected a key set fetched from another origin to fail")
	}
}

func TestSlotsForPairsConfiguredTargetsWithRequestedStates(t *testing.T) {
	targets := []config.Target{{ID: "rider-a"}, {ID: "rider-b"}}

	slots, err := slotsFor(targets, "current, unauthorized")
	if err != nil {
		t.Fatalf("slotsFor() error = %v", err)
	}
	if len(slots) != 2 || slots[0].ID != "rider-a" || slots[1].ID != "rider-b" {
		t.Fatalf("slotsFor() = %+v, want the configured slots in order", slots)
	}
	if slots[0].State != demo.SlotCurrent || slots[1].State != demo.SlotUnauthorized {
		t.Errorf("slotsFor() states = %q, %q", slots[0].State, slots[1].State)
	}
}

// A state list that does not line up with the configured slots would seed a
// target the served surface does not know about, or leave one unseeded.
func TestSlotsForRejectsAMismatchedRequest(t *testing.T) {
	targets := []config.Target{{ID: "rider-a"}}

	if _, err := slotsFor(targets, "current,failed"); err == nil {
		t.Error("expected too many states to be rejected")
	}
	if _, err := slotsFor(targets, "lagging"); err == nil {
		t.Error("expected an unknown state to be rejected")
	}
}

func TestIssuerMatchesHowTheVerifierDerivesIt(t *testing.T) {
	tests := map[string]string{
		"demo":                       "https://demo.cloudflareaccess.com",
		"demo.cloudflareaccess.com":  "https://demo.cloudflareaccess.com",
		"https://demo.example.test/": "https://demo.example.test",
	}

	for domain, want := range tests {
		if got := issuerFor(domain); got != want {
			t.Errorf("issuerFor(%q) = %q, want %q", domain, got, want)
		}
	}
}
