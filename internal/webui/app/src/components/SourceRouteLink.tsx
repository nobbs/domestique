/**
 * The link out to the route a stage was made from.
 *
 * Clearly outbound and clearly not an editing control on this page: it opens the
 * provider's own web application in a new tab, so following it neither loses the
 * stage being read nor implies that anything here writes back. `noreferrer`
 * keeps the private origin the operator is reading on out of the request, which
 * is the same bargain the repository link in the header makes.
 *
 * It names the route, because a route is what the provider can address. A stage
 * is not addressable there — the fragment on one of its URLs is a map viewport,
 * not a stage selector — so an affordance promising stage-level precision would
 * be promising something the destination cannot keep.
 *
 * Nothing at all when there is no link to give, rather than a disabled control
 * or a dead anchor: a deployment with no configured provider base URL has no way
 * back to offer, and saying so with an inert control would be saying it worse.
 */

import { sourceRoute } from "../lib/sourceRoute";

export interface SourceRouteLinkProps {
  /** The provider's web application, as configured. Undefined offers no link. */
  baseUrl: string | undefined;
  /** The provider's own identifier for the route, as stored with the stage. */
  routeId: number;
}

export function SourceRouteLink({ baseUrl, routeId }: SourceRouteLinkProps) {
  const source = sourceRoute(baseUrl, routeId);
  if (!source) {
    return null;
  }

  return (
    <a
      className="route-page__source"
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
       * The label is real text, not decoration: it is what a reader sees, and
       * it is also what remains if the arrow beside it fails to paint.
       */}
      <span>Source route</span>
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
    </a>
  );
}
