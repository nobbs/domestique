import type { Route } from "../../api/types";
import { RouteGlyph } from "../../components/route/RouteGlyph";
import {
  formatAscent,
  formatDistance,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
import { gradientBand } from "../../lib/profile";
import type { RouteChange } from "../../lib/seenRoutes";
import type { UnitSystem } from "../../lib/units";
import { RouteChangeBadge } from "./RouteChangeBadge";
import type { RouteShape } from "./SearchPanel";

/** One route, closed: its shape, its name, and the figures that rank it. */
export function ResultRow({
  route,
  shape,
  change,
  onSelect,
  unitSystem,
}: {
  route: Route;
  shape: RouteShape | undefined;
  change: RouteChange;
  onSelect: () => void;
  unitSystem: UnitSystem;
}) {
  return (
    <li>
      <button
        className="grid w-full grid-cols-[2.5rem_1fr_auto] items-center gap-x-2 gap-y-1 rounded-lg p-2 text-left text-sm hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
        type="button"
        onClick={onSelect}
      >
        <span className="row-span-2 flex size-10 items-center justify-center">
          <RouteGlyph
            coordinates={shape?.coordinates ?? []}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="min-w-0 truncate font-semibold">{route.title}</span>
        <RouteChangeBadge change={change} />
        <span className="col-start-2 col-end-4 flex flex-wrap gap-x-2 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
          <span>{formatDistance(route.distanceMetres, unitSystem)}</span>
          <span>{formatAscent(route.ascentMetres, unitSystem)}</span>
          <span>{formatGradient(route.maxGradientPercent)}</span>
          <span>
            {formatMovingTime(route.movingSeconds)}
            {route.movingSeconds !== undefined && route.validation ? (
              <span className="ml-1">{formatMovingTimeUncertainty(route.validation)}</span>
            ) : null}
          </span>
        </span>
      </button>
    </li>
  );
}
