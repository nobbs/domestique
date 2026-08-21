/**
 * The entry page: a map of everything, and one control over it.
 *
 * The page owns the three things the map and the panel have to agree on — what
 * the library is, what the search left of it, and which route is selected — so
 * neither of them holds a copy. There is no sort and no second presentation:
 * the column is read from the top while the map beside it answers where, and a
 * second arrangement of the same rows would be a second answer to a question the
 * figures in each row already answer.
 *
 * Geometry is fetched per route rather than taken from the listing, because the
 * listing carries no bounding box: the entry map needs a line for every route
 * and the glyphs need the same points, so one query serves both and the shared
 * cache means the route page later opens on geometry that is already here.
 */

import type { UseQueryResult } from "@tanstack/react-query";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { routeGeometryQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { BoundingBox, Position, RouteGeometry } from "../../api/types";
import { routeKey } from "../../api/types";
import { Layout } from "../../components/Layout";
import { ErrorMessage, StatusMessage } from "../../components/StatusMessage";
import { Wordmark } from "../../components/Wordmark";
import { basemapFor, usePrefersDarkScheme } from "../../lib/basemap";
import { formatTimestamp } from "../../lib/format";
import { matchingRoutes } from "../../lib/library";
import type { LibraryLine } from "./LibraryMap";
import { LibraryMap } from "./LibraryMap";
import type { RouteShape } from "./SearchPanel";
import { SearchPanel } from "./SearchPanel";

/** The smallest box every drawn route fits inside, or null for no geometry yet. */
function unionOf(boxes: BoundingBox[]): BoundingBox | null {
  const first = boxes[0];
  if (!first) {
    return null;
  }

  return boxes.reduce<BoundingBox>(
    (total, box) => [
      Math.min(total[0], box[0]),
      Math.min(total[1], box[1]),
      Math.max(total[2], box[2]),
      Math.max(total[3], box[3]),
    ],
    first,
  );
}

export function RoutesPage() {
  const routes = useQuery(routesQuery());
  const config = useQuery(webUIConfigQuery());
  const status = useQuery(statusQuery());
  const prefersDark = usePrefersDarkScheme();

  const [query, setQuery] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const library = useMemo(() => routes.data ?? [], [routes.data]);
  const shown = useMemo(() => matchingRoutes(library, query), [library, query]);

  /*
   * One request per route, in parallel, and each of them cached for as long as
   * the geometry query says. It is the cost of a map of everything: the listing
   * has no bounding box to draw from, and adding one would be a change to the
   * service's wire contract for the sake of a first paint.
   */
  /*
   * Combined here rather than in a memo over the results, because `useQueries`
   * hands back a new array on every render and a memo keyed on it would rebuild
   * the collection every time — and the map would be given new lines to upload
   * every time with it. `combine` is memoised against the results themselves,
   * so the map is handed the same lines until a geometry actually arrives.
   */
  const combine = useCallback(
    (results: Array<UseQueryResult<RouteGeometry>>) => {
      const lines: LibraryLine[] = [];
      const shapes = new Map<string, RouteShape>();
      const boxes = new Map<string, BoundingBox>();
      library.forEach((route, index) => {
        const geometry = results[index]?.data;
        if (!geometry) {
          return;
        }
        const key = routeKey(route);
        const coordinates: Position[] = geometry.coordinates;
        lines.push({ key, coordinates });
        shapes.set(key, { coordinates });
        boxes.set(key, geometry.bbox);
      });

      return { lines, shapes, boxes };
    },
    [library],
  );

  const drawn = useQueries({
    queries: library.map((route) => routeGeometryQuery(route.routeId, route.stageOrder)),
    combine,
  });

  // The camera follows the selection when there is one, because a route picked
  // out of the column is a route the reader now wants to see the shape of.
  // Keyed rather than indexed: geometry arrives one request at a time, so a
  // position in a list of what has arrived is not a position in the library.
  const selectedBox = selectedKey ? (drawn.boxes.get(selectedKey) ?? null) : null;
  const bounds = selectedBox ?? unionOf([...drawn.boxes.values()]);

  const basemap = config.data ? basemapFor(config.data, prefersDark) : null;
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;

  return (
    <Layout
      expanded={query.trim() !== "" || selectedKey !== null}
      map={
        basemap ? (
          <LibraryMap
            styleUrl={basemap.styleUrl}
            darkBasemap={basemap.dark}
            lines={drawn.lines}
            selectedKey={selectedKey}
            bounds={bounds}
          />
        ) : null
      }
    >
      {/*
       * The page's own name. The wordmark below says what the application is
       * rather than what this page shows, so without this the document has no
       * top-level heading at all and a reader navigating by heading is dropped
       * into the middle of a hierarchy. It is not drawn: the map is the title.
       */}
      <h1 className="visually-hidden">Route library</h1>
      <Wordmark />
      {routes.isError ? <ErrorMessage what="the route library" error={routes.error} /> : null}
      {config.isError ? <ErrorMessage what="the map configuration" error={config.error} /> : null}
      {/*
       * Nothing while the library is on its way: the map with no traces on it is
       * the loading state, and a panel saying so would cover the ground it is
       * waiting to draw.
       */}
      {routes.isSuccess && library.length === 0 ? (
        <StatusMessage
          title="No routes yet."
          detail="Routes appear here after the first successful read of the library."
        />
      ) : null}
      {library.length > 0 ? (
        <SearchPanel
          shown={shown}
          total={library.length}
          query={query}
          onQueryChange={(next) => {
            setQuery(next);
            // A search that no longer holds the open route would leave the card
            // expanded off the bottom of a column it is not in.
            setSelectedKey(null);
          }}
          selectedKey={selectedKey}
          onSelect={setSelectedKey}
          shapes={drawn.shapes}
          readAt={readAt ? formatTimestamp(readAt) : null}
        />
      ) : null}
    </Layout>
  );
}
