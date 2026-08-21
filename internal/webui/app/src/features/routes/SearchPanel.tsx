/**
 * The one control on the entry page.
 *
 * At rest it is a pill saying how many routes there are. Typed into, it grows a
 * results column beneath itself; picking a result replaces that row with the
 * route's card, so the column never says the same route twice and nothing opens
 * beside anything else. The card's one button opens the route, which swaps this
 * panel for the route's own — there is no route page to leave for.
 *
 * Narrowing happens here in the browser, over the listing the page already
 * holds — see `lib/library.ts`. Nothing a reader types is sent to the service,
 * which keeps route names out of an access log.
 */

import type { Position, Route } from "../../api/types";
import { routeKey } from "../../api/types";
import { Button } from "../../components/Button";
import { RouteGlyph } from "../../components/RouteGlyph";
import { formatAscent, formatDistance, formatGradient } from "../../lib/format";
import { gradientBand, gradientMix } from "../../lib/profile";

/** The geometry a row needs, when it has arrived. Rows render without it. */
export interface RouteShape {
  coordinates: Position[];
}

export interface SearchPanelProps {
  /** What the search left, in the order it is read. */
  shown: Route[];
  /** The whole library, which is what the pill counts. */
  total: number;
  query: string;
  onQueryChange: (query: string) => void;
  /** The expanded route, by `routeKey`. */
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
  /**
   * Opens a route, which swaps this whole panel for the route's own.
   *
   * Two steps rather than one: a row picked out of the column is a route the
   * reader is looking at on the map, and swapping the search away the moment
   * they touch a row would take the column with it — including the rows they
   * were comparing this one against.
   */
  onOpen: (key: string) => void;
  /** Route shapes by `routeKey`, for the glyphs and the mix bar. */
  shapes: Map<string, RouteShape>;
  /**
   * When the library was last read, for the card's second line.
   *
   * One timestamp for the whole library rather than one per route: the service
   * reads the library in a single pass, so every route in it was read at the
   * same moment and a per-route time would be the same time repeated.
   */
  readAt: string | null;
}

/** The magnifier. Decorative: the field beside it carries the accessible name. */
function SearchIcon() {
  return (
    <svg
      className="search__icon"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      aria-hidden="true"
    >
      <circle cx="7" cy="7" r="4.5" />
      <line x1="10.4" y1="10.4" x2="14" y2="14" strokeLinecap="round" />
    </svg>
  );
}

/** One route, closed: its shape, its name, and the three figures that rank it. */
function ResultRow({
  route,
  shape,
  onSelect,
}: {
  route: Route;
  shape: RouteShape | undefined;
  onSelect: () => void;
}) {
  return (
    <li>
      <button className="result" type="button" onClick={onSelect}>
        <span className="result__glyph">
          <RouteGlyph
            coordinates={shape?.coordinates ?? []}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="result__name">{route.title}</span>
        <span className="result__figures">
          <span>{formatDistance(route.distanceMetres)}</span>
          <span>{formatAscent(route.ascentMetres)}</span>
          <span>{formatGradient(route.maxGradientPercent)}</span>
        </span>
      </button>
    </li>
  );
}

/**
 * One route, opened.
 *
 * Where it is, is the route's own name where that is not already the title: the
 * service stores no locality, and asking a geocoder for one would send the
 * library's coordinates outside the Tailnet to answer a question the operator's
 * own naming already answers.
 */
function RouteCard({
  route,
  shape,
  readAt,
  onOpen,
}: {
  route: Route;
  shape: RouteShape | undefined;
  readAt: string | null;
  onOpen: () => void;
}) {
  const where = route.routeName !== route.title ? route.routeName : null;
  const second = [where, readAt ? `read ${readAt}` : null].filter(Boolean).join(" · ");
  const mix = shape ? gradientMix(shape.coordinates) : [];
  // A run's place along the route is its identity — two runs of the same band
  // are the same band at different kilometres — so the key is where the run
  // starts rather than which band it is.
  let offset = 0;
  const runs = mix.map((entry) => {
    const start = offset;
    offset += entry.share;

    return { ...entry, start };
  });

  return (
    <li className="route-card">
      <h2 className="route-card__title">{route.title}</h2>
      {second === "" ? null : <p className="route-card__where">{second}</p>}
      <dl className="route-card__figures">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres)}</dd>
        </div>
        <div>
          <dt>Climbing</dt>
          <dd>{formatAscent(route.ascentMetres)}</dd>
        </div>
        <div>
          <dt>Max</dt>
          <dd>{formatGradient(route.maxGradientPercent)}</dd>
        </div>
      </dl>
      {mix.length > 0 ? (
        // Decorative: every band in it is stated in the key on the route's own
        // page, and a reader who cannot see the colours loses nothing here that
        // the three figures above have not already said.
        <div className="route-card__mix" aria-hidden="true">
          {runs.map((entry) => (
            <span
              key={`${entry.start.toFixed(6)}-${entry.band}`}
              data-band={entry.band}
              style={{ width: `${(entry.share * 100).toFixed(3)}%` }}
            />
          ))}
        </div>
      ) : null}
      {/*
       * The one primary control in this view. It opens the route in place —
       * there is no route page to go to — so it says what it will show rather
       * than where it will take anybody.
       */}
      <Button className="route-card__open" variant="primary" onClick={onOpen}>
        Open route
      </Button>
    </li>
  );
}

export function SearchPanel({
  shown,
  total,
  query,
  onQueryChange,
  selectedKey,
  onSelect,
  onOpen,
  shapes,
  readAt,
}: SearchPanelProps) {
  const expanded = query.trim() !== "" || selectedKey !== null;

  return (
    <div className="panel search">
      <div className="search__pill">
        <SearchIcon />
        <input
          className="search__field"
          type="search"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder={`Search ${total} ${total === 1 ? "route" : "routes"}`}
          aria-label="Search the route library"
        />
        {/*
         * Derived from the filter rather than written anywhere: the count and
         * the column are the same fact, so they cannot disagree. It says nothing
         * until the library has been narrowed, because "47 of 47" is a sum with
         * no question behind it.
         */}
        {query.trim() === "" ? null : (
          <span className="search__count">
            {shown.length} of {total}
          </span>
        )}
      </div>
      {expanded && shown.length === 0 ? (
        <p className="search__empty">Nothing here is called that.</p>
      ) : null}
      {expanded && shown.length > 0 ? (
        <ul className="search__results">
          {shown.map((route) => {
            const key = routeKey(route);
            const shape = shapes.get(key);

            return key === selectedKey ? (
              <RouteCard
                key={key}
                route={route}
                shape={shape}
                readAt={readAt}
                onOpen={() => onOpen(key)}
              />
            ) : (
              <ResultRow key={key} route={route} shape={shape} onSelect={() => onSelect(key)} />
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}
