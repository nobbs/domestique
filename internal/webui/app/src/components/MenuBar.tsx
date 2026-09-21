/**
 * The application's one navigation: a bar across the top of every page.
 *
 * It used to be a pill floating over the corner of the map, on the argument
 * that a bar across the top of a map is a bar across the map. That was true of
 * a bar drawn *over* the cartography; this one is drawn above it, and the map
 * begins underneath. The pill's cost was that it could hold a name and two
 * marks and nothing else, so the two pages it did not reach — sync and
 * settings — each grew a back-link and a heading of their own, and the
 * application had three headers and no navigation.
 *
 * The links are named rather than drawn, because a bar has the room the pill
 * did not and a glyph is only ever a guess at a word. Where the room runs out —
 * a phone, a narrow window, the admin link arriving — the names that do not fit
 * move into a menu at the end of the row rather than being shortened to marks
 * or allowed to push the row off the screen. Which names those are is measured
 * rather than declared at a breakpoint, so every name the width can hold is
 * still a name.
 *
 * Navigation runs from the left, and the colour scheme and the session sit at
 * the right end, with the gap between them doing the separating: where a reader
 * can go and which session they are in are two different questions, and a row
 * that answers both in one run of items invites the second to be read as a
 * third destination. The account page, and what sync is doing, sit with the
 * session.
 */

import { IconChevronDown } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { matchPath, NavLink, useLocation } from "react-router";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { webUIConfigQuery } from "../api/queries";
import { useEffectiveAdmin } from "../lib/identity";
import { useFittingCount } from "../lib/useFittingCount";
import { Wordmark } from "./brand/Wordmark";
import { RouteJump } from "./RouteJump";
import { ThemeToggle } from "./ThemeToggle";
import { UserPill } from "./UserPill";

interface Destination {
  readonly to: string;
  readonly label: string;
  readonly end: boolean;
}

/** The landing page: `/` redirects here. */
const ACTIVITIES_DESTINATION: Destination = { to: "/activities", label: "Activities", end: false };

const CATALOGUE_DESTINATION: Destination = { to: "/catalogue", label: "Catalogue", end: false };

/** After the catalogue: shown only where the planner exists — an admin and a routing engine. */
const PLAN_DESTINATION: Destination = { to: "/plan", label: "Plan", end: false };

const REST_DESTINATIONS: readonly Destination[] = [
  { to: "/fitness", label: "Fitness", end: false },
];

const ADMIN_DESTINATION: Destination = { to: "/admin", label: "Admin", end: false };

/** The rule `NavLink` paints itself by, asked here so the measurement can mirror it. */
function isCurrent({ to, end }: Destination, pathname: string): boolean {
  return matchPath({ path: to, end }, pathname) !== null;
}

/** Whether the page being read is one of these. */
function holdsCurrent(destinations: readonly Destination[], pathname: string): boolean {
  return destinations.some((destination) => isCurrent(destination, pathname));
}

/** Which page you are on is `aria-current`, which paints the link's own text. */
const LINK_CLASS =
  "relative inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md px-2 py-1.5 text-sm text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)] aria-[current=page]:font-semibold aria-[current=page]:text-[var(--ink)]";

/**
 * The same shape as a link, because it stands in the same row and leads to the
 * same kind of place. `data-holds-current` is the one thing a link says that a
 * button cannot: `aria-current` would claim this control is the page.
 */
const TRIGGER_CLASS =
  "relative inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md px-2 py-1.5 text-sm text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)] data-holds-current:font-semibold data-holds-current:text-[var(--ink)] data-popup-open:bg-[var(--base)] data-popup-open:text-[var(--ink)]";

export function MenuBar() {
  const { data: config } = useQuery(webUIConfigQuery());
  const effectiveAdmin = useEffectiveAdmin();
  const destinations = [
    ACTIVITIES_DESTINATION,
    CATALOGUE_DESTINATION,
    ...(config?.planning && effectiveAdmin ? [PLAN_DESTINATION] : []),
    ...REST_DESTINATIONS,
    ...(effectiveAdmin ? [ADMIN_DESTINATION] : []),
  ];
  const { pathname } = useLocation();
  const { frameRef, measureRef, visible } = useFittingCount(destinations.length);
  const shown = destinations.slice(0, visible);
  const folded = destinations.slice(visible);

  return (
    <header className="sticky top-0 z-40 flex h-[calc(3rem+env(safe-area-inset-top))] shrink-0 items-center gap-3 border-[var(--rule)] border-b bg-[var(--panel)] px-3 pt-[env(safe-area-inset-top)] sm:h-[calc(3.5rem+env(safe-area-inset-top))] sm:gap-6 sm:px-4">
      <Wordmark className="shrink-0" />
      {/* The row is what is left of the bar after the brand and the session, and
          it keeps that width whatever it holds — which is what stops a fold from
          changing the budget that decided it. It clips, or the measured copy
          below would give the page a scrollbar the width of every name at once;
          the clip is held off the edge by the focus ring's offset, which is
          painted outside the box it belongs to and would otherwise go with it. */}
      <nav
        aria-label="Primary"
        className="relative flex min-w-0 flex-1 items-center gap-0.5 overflow-clip [overflow-clip-margin:4px] sm:gap-1"
        ref={frameRef}
      >
        {/* Every name at its natural width, laid out but never painted, so the
            arithmetic can still see the ones the row has folded away. The weight
            `aria-current` adds is mirrored here, because a variable font's bold
            is wider than its regular and a measurement that missed that would
            fold a name the row has room for. */}
        <div
          aria-hidden="true"
          className="pointer-events-none invisible absolute top-0 left-0 flex items-center gap-0.5 sm:gap-1"
          data-slot="overflow-measure"
          ref={measureRef}
        >
          {destinations.map((destination) => (
            <span
              className={cn(LINK_CLASS, isCurrent(destination, pathname) && "font-semibold")}
              key={destination.to}
            >
              {destination.label}
            </span>
          ))}
          {/* Measured carrying the current-page weight whether or not the real
              one will: it depends on which names end up folded, and a
              measurement that moved with its own answer would never settle. */}
          <span className={TRIGGER_CLASS} data-holds-current="true">
            More
            <IconChevronDown aria-hidden="true" size={14} stroke={1.6} />
          </span>
        </div>
        {shown.map(({ to, label, end }) => (
          <NavLink key={to} to={to} end={end} className={LINK_CLASS}>
            {label}
          </NavLink>
        ))}
        {folded.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              className={TRIGGER_CLASS}
              // Not `aria-current`: the reader's page is in here, but this
              // control is not it.
              data-holds-current={holdsCurrent(folded, pathname) || undefined}
            >
              More
              <IconChevronDown aria-hidden="true" size={14} stroke={1.6} />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-auto min-w-36">
              {folded.map(({ to, label, end }) => (
                <DropdownMenuItem
                  className="aria-[current=page]:font-semibold aria-[current=page]:text-[var(--ink)]"
                  key={to}
                  // An anchor underneath, so middle-click and copy-link still
                  // mean what they mean everywhere else in the row.
                  render={<NavLink to={to} end={end} />}
                >
                  {label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </nav>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <RouteJump />
        <ThemeToggle />
        <UserPill />
      </div>
    </header>
  );
}
