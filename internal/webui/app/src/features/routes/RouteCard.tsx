import { useEffect, useRef } from "react";
import type { Route } from "../../api/types";
import { Button } from "../../components/Button";
import {
  formatAscent,
  formatDistance,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
import { gradientMix } from "../../lib/profile";
import { providerLabel } from "../../lib/provider";
import type { StageChange } from "../../lib/seenStages";
import type { UnitSystem } from "../../lib/units";
import type { RouteShape } from "./SearchPanel";
import { StageChangeBadge } from "./StageChangeBadge";

/**
 * One route, opened.
 *
 * Where it is, is the route's own name where that is not already the title: the
 * service stores no locality, and asking a geocoder for one would send the
 * library's coordinates outside the Tailnet to answer a question the operator's
 * own naming already answers.
 */
export function RouteCard({
  route,
  shape,
  readAt,
  change,
  onOpen,
  unitSystem,
}: {
  route: Route;
  shape: RouteShape | undefined;
  readAt: string | null;
  change: StageChange;
  onOpen: () => void;
  unitSystem: UnitSystem;
}) {
  /*
   * The card brings itself into view when it appears.
   *
   * A route can now be picked by pointing at it on the map, and the column it
   * expands in is not necessarily scrolled anywhere near it: without this, a
   * click on the ground would answer somewhere the reader cannot see. `nearest`
   * so a card picked out of the column, which is already on screen, does not
   * make the column jump under the hand that picked it.
   */
  const card = useRef<HTMLLIElement>(null);
  useEffect(() => {
    card.current?.scrollIntoView({ block: "nearest" });
  }, []);

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
    <li className="rounded-lg border border-[var(--rule)] bg-[var(--base)] p-3" ref={card}>
      <h2 className="text-base font-semibold">{route.title}</h2>
      <StageChangeBadge change={change} />
      <span className="ml-1 text-xs font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {providerLabel(route.provider)}
      </span>
      {second === "" ? null : <p className="mt-1 text-sm text-[var(--ink-2)]">{second}</p>}
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Climbing</dt>
          <dd>{formatAscent(route.ascentMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Max</dt>
          <dd>{formatGradient(route.maxGradientPercent)}</dd>
        </div>
        <div>
          <dt>Moving time</dt>
          <dd>
            {formatMovingTime(route.movingSeconds)}
            {route.movingSeconds !== undefined && route.validation ? (
              <span className="ml-1 text-xs text-[var(--ink-2)]">
                {formatMovingTimeUncertainty(route.validation)}
              </span>
            ) : null}
          </dd>
        </div>
      </dl>
      {mix.length > 0 ? (
        // Decorative: every band in it is stated in the key on the route's own
        // page, and a reader who cannot see the colours loses nothing here that
        // the three figures above have not already said.
        <div
          className="mt-3 flex h-1.5 overflow-hidden rounded-sm"
          data-testid="gradient-mix"
          aria-hidden="true"
        >
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
      <Button className="mt-3" variant="primary" onClick={onOpen}>
        Open route
      </Button>
    </li>
  );
}
