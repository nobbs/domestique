/**
 * **B — Terrain.** Where the hard ground is, rather than how much of it there is.
 *
 * Taking the chart out of this panel takes something with it that the figures
 * never carried: the strip along its foot, which said where the steep
 * kilometres and the loose ground actually fall. A route that is a fifth
 * six-percent reads very differently depending on whether that fifth is one
 * col or scattered through eighty kilometres of rollers, and no mix bar can
 * tell those apart — a bar totals the route and throws the order away.
 *
 * So this draws both mixes in ride order on one shared axis, steepness over
 * ground, with the cols bracketed above them. Two ribbons and an axis come to
 * about fifty pixels, against the chart's hundred and twenty, and recover most
 * of what a reader was actually reading the chart for at a glance.
 *
 * The bet is that "where" beats "how much" once there is no chart to ask.
 * The cost is that the ribbons are not a control worth aiming at: a run of
 * three hundred metres on a hundred and thirty kilometre route is a segment
 * half a pixel wide. The class list beneath is the reliable way to press one,
 * and the ribbon is what makes the answer legible once pressed.
 */

import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
} from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import { ticksFor } from "../../../lib/profile";
import { distanceUnitLabel, distanceValue } from "../../../lib/units";
import type { CardProps, MixEntry } from "./shared";
import {
  bandEntries,
  CardHeading,
  climbSentence,
  formatShare,
  gradientSegments,
  groundSegments,
  HighlightToggle,
  Ribbon,
  surfaceEntries,
} from "./shared";

/** The classes actually present, as the reliable press targets. */
function ClassList({
  name,
  entries,
  absence,
  highlight,
  onHighlightChange,
}: {
  name: string;
  entries: MixEntry[];
  absence: string | null;
  highlight: Highlight | null;
  onHighlightChange: (next: Highlight | null) => void;
}) {
  if (entries.length === 0) {
    return <p className="text-xs text-[var(--ink-2)]">{absence}</p>;
  }

  return (
    <ul className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <li className="text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {name}
      </li>
      {entries.map((entry) => (
        <li key={entry.label}>
          <HighlightToggle
            highlight={entry.highlight}
            current={highlight}
            onChange={onHighlightChange}
            label={`${entry.label}, ${entry.description}, ${formatShare(entry.share)} of the route`}
            title={entry.description}
            className="flex items-center gap-1 rounded px-1 py-0.5 text-xs hover:bg-[var(--base)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)]"
          >
            <span
              className="size-2 shrink-0 rounded-[2px]"
              style={{ background: entry.colour }}
              aria-hidden="true"
            />
            <span className="text-[var(--ink-2)]">{entry.label}</span>
            <span className="font-semibold tabular-nums">{formatShare(entry.share)}</span>
          </HighlightToggle>
        </li>
      ))}
    </ul>
  );
}

function Figure({ value, unit }: { value: string; unit: string }) {
  return (
    <span className="whitespace-nowrap">
      <span className="font-semibold tabular-nums">{value}</span>{" "}
      <span className="text-xs text-[var(--ink-2)]">{unit}</span>
    </span>
  );
}

export function TerrainCard({
  route,
  subtitle,
  movingSeconds,
  highestMetres,
  bands,
  runs,
  surface,
  surfaceAbsence,
  climbs,
  highlight,
  onHighlightChange,
  unitSystem,
}: CardProps) {
  const total = route.distanceMetres;
  const climbLine = climbSentence(climbs, unitSystem);

  // The same ticks the chart's own distance axis picks, so a reader who has
  // seen one recognises the other.
  const ticks = ticksFor(0, distanceValue(total, unitSystem), 4);
  const axisMax = distanceValue(total, unitSystem);

  return (
    <div className="grid gap-3">
      <CardHeading route={route} subtitle={subtitle} />
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
        <Figure value={formatDistance(total, unitSystem)} unit="" />
        <Figure value={formatAscent(route.ascentMetres, unitSystem)} unit="up" />
        <Figure value={formatGradient(route.maxGradientPercent)} unit="max" />
        <Figure value={formatMovingTime(movingSeconds)} unit="moving" />
        <Figure
          value={highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}
          unit="high"
        />
      </div>
      <div>
        {/*
         * The cols, bracketed over the ribbons they cover. Numbered rather than
         * named: three brackets over a hundred and thirty kilometres have room
         * for an ordinal and nothing else, and the line below says which one
         * matters.
         */}
        <div className="relative mb-1 h-3.5">
          {climbs.map((climb, index) => (
            <span
              key={climb.startMetres}
              className="absolute top-2 h-1 rounded-full bg-[var(--ink-2)]"
              style={{
                left: `${(climb.startMetres / total) * 100}%`,
                width: `${((climb.endMetres - climb.startMetres) / total) * 100}%`,
              }}
            >
              <span className="absolute -top-2.5 left-0 text-[10px] leading-none text-[var(--ink-2)] tabular-nums">
                {index + 1}
              </span>
            </span>
          ))}
        </div>
        <div className="grid gap-0.5">
          <Ribbon segments={gradientSegments(runs)} className="h-4" highlight={highlight} />
          <Ribbon segments={groundSegments(surface)} className="h-2.5" highlight={highlight} />
        </div>
        <div className="relative mt-0.5 h-3.5">
          {ticks.map((tick) => (
            <span
              key={tick}
              className="absolute top-0 text-[10px] leading-none text-[var(--ink-2)] tabular-nums"
              style={{
                left: `${axisMax > 0 ? (tick / axisMax) * 100 : 0}%`,
                // The last tick would otherwise hang off the right edge, and
                // the first would sit half outside the ribbon it labels.
                transform:
                  tick === 0 ? "none" : tick >= axisMax ? "translateX(-100%)" : "translateX(-50%)",
              }}
            >
              {tick === 0 ? `0 ${distanceUnitLabel(unitSystem)}` : tick}
            </span>
          ))}
        </div>
      </div>
      {climbLine === null ? null : <p className="-mt-1 text-xs text-[var(--ink-2)]">{climbLine}</p>}
      <div className="grid gap-1">
        <ClassList
          name="Gradient"
          entries={bandEntries(bands)}
          absence="No elevation data."
          highlight={highlight}
          onHighlightChange={onHighlightChange}
        />
        <ClassList
          name="Surface"
          entries={surfaceEntries(surface)}
          absence={surfaceAbsence}
          highlight={highlight}
          onHighlightChange={onHighlightChange}
        />
      </div>
    </div>
  );
}
