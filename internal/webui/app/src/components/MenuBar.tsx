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
 * did not and a glyph is only ever a guess at a word.
 *
 * Navigation runs from the left and the session sits at the right end, with the
 * gap between them doing the separating: where a reader can go and which
 * session they are in are two different questions, and a row that answers both
 * in one run of items invites the second to be read as a third destination.
 */

import { useQuery } from "@tanstack/react-query";
import { NavLink } from "react-router";
import { statusQuery } from "../api/queries";
import { syncState } from "../lib/syncState";
import { Wordmark } from "./brand/Wordmark";
import { UserPill } from "./UserPill";

/**
 * Which page a link leads to, and whether it is the page being read.
 *
 * `end` only on the map, whose path is a prefix of nothing but is matched by
 * everything without it.
 */
const DESTINATIONS = [
  { to: "/", label: "Atlas", end: true },
  { to: "/sync", label: "Sync", end: false },
  { to: "/settings", label: "Settings", end: false },
] as const;

/*
 * Two channels, kept apart on purpose. Which page you are on is `aria-current`,
 * which paints the link's own text; what synchronisation is doing is a dot
 * beside the word, which paints nothing else. A reader who met both as a colour
 * on the same word could not tell "you are here" from "something needs you".
 */
const LINK_CLASS =
  "relative inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)] aria-[current=page]:font-semibold aria-[current=page]:text-[var(--ink)]";

export function MenuBar() {
  const { data } = useQuery(statusQuery());
  const state = data ? syncState(data) : null;
  const described = state ? `Sync · ${state.label}` : undefined;

  return (
    <header className="sticky top-0 z-40 flex h-[calc(3rem+env(safe-area-inset-top))] shrink-0 items-center gap-3 border-[var(--rule)] border-b bg-[var(--panel)] px-3 pt-[env(safe-area-inset-top)] sm:h-[calc(3.5rem+env(safe-area-inset-top))] sm:gap-6 sm:px-4">
      <Wordmark />
      <nav aria-label="Primary" className="flex items-center gap-0.5 sm:gap-1">
        {DESTINATIONS.map(({ to, label, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={LINK_CLASS}
            /*
             * The state is spelled out for anyone the dot says nothing to, and
             * the name still begins with the word on screen so that saying
             * "Sync" to a voice control reaches this link. A state that has not
             * arrived leaves the name alone: a status request failing is not the
             * reader's problem until they ask about synchronisation.
             */
            aria-label={to === "/sync" ? described : undefined}
            title={to === "/sync" ? described : undefined}
            data-tone={to === "/sync" ? state?.tone : undefined}
          >
            {label}
            {to === "/sync" && state?.tone ? (
              <span
                className="size-1.5 rounded-full bg-[var(--ink-2)] data-[tone=alert]:bg-[var(--alert)] data-[tone=good]:bg-[var(--good)] data-[tone=hold]:bg-[var(--hold)]"
                data-tone={state.tone}
                aria-hidden="true"
              />
            ) : null}
          </NavLink>
        ))}
      </nav>
      <div className="ml-auto">
        <UserPill />
      </div>
    </header>
  );
}
