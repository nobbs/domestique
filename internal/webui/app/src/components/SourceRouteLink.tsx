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
 * the menu it sits in. The spoken name is the one that promises precision, and
 * it promises route-level precision only: a stage is not addressable at the
 * provider — the fragment on one of its URLs is a map viewport, not a stage
 * selector — so an affordance offering more would be offering something the
 * destination cannot keep.
 *
 * Nothing at all when there is no link to give, rather than a disabled control
 * or a dead anchor: a deployment with no configured provider base URL has no way
 * back to offer, and saying so with an inert control would be saying it worse.
 */

import { IconExternalLink } from "@tabler/icons-react";
import { sourceRoute } from "../lib/sourceRoute";
import { DropdownMenuItem } from "./ui/dropdown-menu";

export interface SourceRouteLinkProps {
  /** The stage's own source, as its stage identity names it. */
  provider: string;
  /** That provider's web application, as configured. Undefined offers no link. */
  baseUrl: string | undefined;
  /** The provider's own identifier for the route, as stored with the stage. */
  sourceRouteId: number;
}

export function SourceRouteLink({ provider, baseUrl, sourceRouteId }: SourceRouteLinkProps) {
  const source = sourceRoute(provider, baseUrl, sourceRouteId);
  if (!source) {
    return null;
  }

  return (
    <DropdownMenuItem
      // A menu item that happens to navigate, so it answers to the menu's own
      // keyboard rather than to Tab. It is still an anchor underneath, which is
      // what keeps middle-click, copy-link and the browser's own idea of an
      // outbound address working.
      render={
        <a
          href={source.href}
          target="_blank"
          rel="noreferrer"
          // Where it goes and what it opens, spoken as part of the name rather
          // than left to a title a keyboard never sees. The route number is in
          // it because it is what the destination is addressed by, and the host
          // because a reader deserves to know a link leaves before they follow
          // it.
          aria-label={`Open source route ${sourceRouteId} on ${source.host} in a new tab`}
        />
      }
    >
      <IconExternalLink aria-hidden="true" />
      {/*
       * The provider names itself. "Source route" described the link's job,
       * which the menu it sits in already says; the destination is the thing a
       * reader cannot work out from context, and it is also the thing that
       * tells them whether following it is worth the tab.
       */}
      {source.name}
    </DropdownMenuItem>
  );
}
