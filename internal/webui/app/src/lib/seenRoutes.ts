/**
 * Which route revisions this reader has already looked at, remembered locally.
 *
 * A route is "new" until the reader opens it once, and "updated" again
 * whenever its safe source revision moves on from the one they last saw — the
 * same revision the service already uses to decide a target needs rewriting.
 * This never reaches the service: it exists only to tell a returning reader
 * which cards differ from the library they last read, and is kept out of the
 * address for the same reason the basemap choice is — it is how one reader
 * likes to read their own library, not something worth sending to somebody
 * else.
 */

import { useCallback, useState } from "react";
import type { Route } from "../api/types";
import { routeKey } from "../api/types";

/** Namespaced the same way the basemap choice is. */
const STORAGE_KEY = "domestique.seen-routes";

export type RouteChange = "new" | "updated" | null;

type SeenRevisions = Record<string, string>;

type Seen = Pick<Route, "provider" | "sourceRouteId" | "stageOrder" | "sourceRevision">;

/**
 * What a reader has already seen, and the one way to update it.
 *
 * markSeen is the sole write, and is called from exactly the moment a route's
 * own panel is shown — never from rendering a card in the list — so looking at
 * the library is never itself the trigger, only opening a route is.
 */
export function useSeenRoutes(): {
  changeOf(route: Seen): RouteChange;
  markSeen(route: Seen): void;
} {
  const [seen, setSeen] = useState<SeenRevisions>(readSeen);

  const changeOf = useCallback(
    (route: Seen): RouteChange => {
      const key = routeKey(route);
      const last = seen[key];
      if (last === undefined) {
        return "new";
      }

      return last === route.sourceRevision ? null : "updated";
    },
    [seen],
  );

  const markSeen = useCallback((route: Seen) => {
    const key = routeKey(route);
    setSeen((current) => {
      if (current[key] === route.sourceRevision) {
        return current;
      }
      const next = { ...current, [key]: route.sourceRevision };
      writeSeen(next);

      return next;
    });
  }, []);

  return { changeOf, markSeen };
}

function readSeen(): SeenRevisions {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);

    return raw ? parseSeen(raw) : {};
  } catch {
    return {};
  }
}

function parseSeen(raw: string): SeenRevisions {
  try {
    const parsed: unknown = JSON.parse(raw);

    return isSeenRevisions(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function isSeenRevisions(value: unknown): value is SeenRevisions {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    Object.values(value).every((entry) => typeof entry === "string")
  );
}

function writeSeen(next: SeenRevisions): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    // Remembering is the whole of what is lost, and the marks made this
    // session still stand for as long as the page is open.
  }
}
