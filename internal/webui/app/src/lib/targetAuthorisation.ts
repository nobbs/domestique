/**
 * What one Wahoo account's authorisation state means, and how it is left.
 *
 * The service serves four words and the page has to turn them into an operator
 * decision, because three of the four are only ever fixed the same way: by one
 * visit to the protected OAuth flow. Before this, the page said "Not connected"
 * for all of them and named no way to that flow, which left the operator with a
 * fact and no next move.
 *
 * `pending` is the reason this is a module rather than a label table. It is a
 * flow the operator has already started and not finished, and it is the one
 * state where the right affordance is none at all: starting a second flow
 * invalidates the first, so an account midway through connecting deliberately
 * offers nothing to press.
 *
 * Every string here is a constant. Nothing from the service is interpolated
 * into one, so this module cannot become a way for a token, an account
 * identity, or an upstream message to reach the page.
 */

/**
 * The authorisation words the service serves, in its own wire names.
 *
 * Mirrors the constants in `internal/httpapi/routes_status.go`. Three are
 * stored on the target slot; `pending` is derived from an OAuth transaction
 * that has neither expired nor been consumed, which is why it can appear and
 * disappear without anything about the account having changed.
 */
export const TARGET_AUTHORISATIONS = [
  "not_authorized",
  "pending",
  "authorized",
  "needs_reauthorization",
] as const;

export type TargetAuthorisation = (typeof TARGET_AUTHORISATIONS)[number];

export interface AuthorisationGuidance {
  /** The word this account gets, in place of the convergence one. */
  label: string;
  /** What the state is and what happens next, in operator language. */
  detail: string;
  /**
   * The link text that starts the protected OAuth flow. Absent when starting
   * one is not the next move — either because a flow is already running, or
   * because this build does not recognise the state it is looking at.
   */
  action?: string;
}

/** Every state except `authorized`, which needs nothing said about it. */
const GUIDANCE: Record<Exclude<TargetAuthorisation, "authorized">, AuthorisationGuidance> = {
  not_authorized: {
    label: "Not connected",
    detail: "This account has never been connected to Wahoo. Nothing is written to it until it is.",
    action: "Connect",
  },
  pending: {
    label: "Connecting",
    detail:
      "A connection was started and has not come back yet. Finish it in the Wahoo tab it opened; it expires ten minutes after it was started, and this account then reads as it did before.",
  },
  needs_reauthorization: {
    label: "Reconnect needed",
    detail:
      "Wahoo stopped accepting this service's authorisation. Nothing was deleted and nothing has been lost — the account has to be connected again before it can be written to.",
    action: "Reconnect",
  },
};

/**
 * A word this build has not heard of.
 *
 * It degrades toward asking the operator to look, the same direction sync
 * guidance degrades in, and offers no action: a flow started against a state
 * this page cannot explain is a guess, and this one is not free to guess with
 * — it invalidates whatever flow is already in progress.
 */
const UNRECOGNISED: AuthorisationGuidance = {
  label: "Connection state unknown",
  detail:
    "The service reported an authorisation state this page does not recognise, so it may be older than the service it is talking to. Check the service's own status output for the word it gave.",
};

function isRecognised(value: string): value is TargetAuthorisation {
  return (TARGET_AUTHORISATIONS as readonly string[]).includes(value);
}

/**
 * What one account's authorisation should tell the operator, or `undefined`
 * when it is connected and there is nothing to act on.
 */
export function authorisationGuidance(authorisation: string): AuthorisationGuidance | undefined {
  // Recognition first, so what is left is one of the four words and the
  // compiler can hold this to covering all of them.
  if (!isRecognised(authorisation)) {
    return UNRECOGNISED;
  }
  if (authorisation === "authorized") {
    return undefined;
  }

  return GUIDANCE[authorisation];
}

/**
 * Where the protected OAuth flow for one account starts.
 *
 * A full-page navigation to the service's own path, deliberately: the flow
 * leaves for Wahoo and comes back to this application, so it is not something a
 * client-side route or a background request can carry.
 */
export function authorisationStartHref(targetId: string): string {
  return `/oauth/wahoo/start/${encodeURIComponent(targetId)}`;
}
