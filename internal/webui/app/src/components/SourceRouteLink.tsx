/**
 * The link out to the route a stage was made from.
 *
 * Clearly outbound and clearly not an editing control on this page: it opens the
 * provider's own web application in a new tab, so following it neither loses the
 * stage being read nor implies that anything here writes back. `noreferrer`
 * keeps the private origin the operator is reading on out of the request, which
 * is the same bargain the repository link in the header makes.
 *
 * The label is the provider, because that is what a reader cannot work out from
 * the row it sits in. The spoken name is the one that promises precision, and
 * it promises route-level precision only: a stage is not addressable at the
 * provider — the fragment on one of its URLs is a map viewport, not a stage
 * selector — so an affordance offering more would be offering something the
 * destination cannot keep.
 *
 * Nothing at all when there is no link to give, rather than a disabled control
 * or a dead anchor: a deployment with no configured provider base URL has no way
 * back to offer, and saying so with an inert control would be saying it worse.
 */

import { sourceRoute } from "../lib/sourceRoute";
import { ExternalButtonLink } from "./Button";

export interface SourceRouteLinkProps {
  /** The stage's own source, as its stage identity names it. */
  provider: string;
  /** That provider's web application, as configured. Undefined offers no link. */
  baseUrl: string | undefined;
  /** The provider's own identifier for the route, as stored with the stage. */
  routeId: number;
}

export function SourceRouteLink({ provider, baseUrl, routeId }: SourceRouteLinkProps) {
  const source = sourceRoute(provider, baseUrl, routeId);
  if (!source) {
    return null;
  }

  return (
    <ExternalButtonLink
      className="route-panel__source"
      href={source.href}
      target="_blank"
      rel="noreferrer"
      // Where it goes and what it opens, spoken as part of the name rather than
      // left to a title a keyboard never sees. The route number is in it because
      // it is what the destination is addressed by, and the host because a
      // reader deserves to know a link leaves before they follow it.
      aria-label={`Open source route ${routeId} on ${source.host} in a new tab`}
    >
      {/*
       * The provider names itself. "Source route" described the link's job,
       * which the row it sits in already says; the destination is the thing a
       * reader cannot work out from context, and it is also the thing that
       * tells them whether following it is worth the tab.
       *
       * Real text, not decoration: it is what remains if the arrow beside it
       * fails to paint.
       */}
      <span>{source.name}</span>
      <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true" focusable="false">
        <path
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M6.5 3H3.8A.8.8 0 0 0 3 3.8v8.4a.8.8 0 0 0 .8.8h8.4a.8.8 0 0 0 .8-.8V9.5M9.5 2.5H13.5V6.5M13 3 7.5 8.5"
        />
      </svg>
    </ExternalButtonLink>
  );
}
