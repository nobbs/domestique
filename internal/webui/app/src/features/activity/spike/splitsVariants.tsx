/**
 * Four positions on what should stand where "By the kilometre" stands today
 * on the ride page (#646), plus the panel itself as a baseline to compare
 * against. Each variant's own note states the bet it makes; see
 * `SplitsSpike.stories.tsx`.
 *
 * Storybook only: nothing here is imported by the application.
 */

import { useCallback, useMemo, useRef } from "react";
import type { ActivitySplit, RouteClimb, RouteClimbAttempt } from "../../../api/types";
import { formatAscent, formatDuration, formatKilometres, formatSpeed } from "../../../lib/format";
import {
  cumulativeMetres,
  elevationOf,
  GRADIENT_BANDS,
  gradientRanges,
} from "../../../lib/profile";
import { RideSplits } from "../RideSplits";
import type { SplitsRideFixture } from "./splitsData";
import { stretchStats } from "./splitsData";

/** The panel this stretch of the page carries today, for the Baseline story. */
export function Baseline({ ride }: { ride: SplitsRideFixture }) {
  return <RideSplits splits={ride.splits} />;
}

/* --------------------------------------------------------------- A · Drop */

/** A · The region is simply absent. The test it leaves behind. */
export function DropRegion({ ride }: { ride: SplitsRideFixture }) {
  return (
    <div className="rounded-xl border border-[var(--rule)] border-dashed p-4 text-[var(--ink-2)] text-xs">
      Nothing stands here for {ride.label.toLowerCase()} — the series chart, the effort panel and
      the profile above already say everything this stretch of the page would have.
    </div>
  );
}

/* --------------------------------------------------------- B · By the climb */

function EmptyClimbNote({ text }: { text: string }) {
  return (
    <section
      className="flex flex-col gap-2 rounded-xl bg-[var(--panel)] p-4 text-[var(--ink-2)] text-xs ring-1 ring-black/5"
      aria-label="By the climb"
    >
      <h2 className="font-medium text-[var(--ink)] text-sm">By the climb</h2>
      <p>{text}</p>
    </section>
  );
}

function attemptPower(attempt: RouteClimbAttempt): string {
  if (attempt.powerWatts !== undefined) {
    return `${Math.round(attempt.powerWatts)} W`;
  }
  if (attempt.estimatedPowerWatts !== undefined) {
    return `~${Math.round(attempt.estimatedPowerWatts)} W`;
  }
  return "—";
}

function ClimbRow({
  climb,
  ordinal,
  activityId,
}: {
  climb: RouteClimb;
  ordinal: number;
  activityId: number;
}) {
  const rank = climb.attempts.findIndex((attempt) => attempt.activityId === activityId);
  const mine = rank >= 0 ? climb.attempts[rank] : undefined;
  const best = climb.attempts[0];

  return (
    <div className="flex items-start justify-between gap-4 border-[var(--rule)] border-t pt-2 text-sm first:border-t-0 first:pt-0">
      <div>
        <div className="font-medium">Climb {ordinal}</div>
        <div className="text-[var(--ink-2)] text-xs tabular-nums">
          {formatKilometres(climb.distanceMetres)} · {climb.averageGradePercent.toFixed(1)}% avg ·{" "}
          {climb.maxGradePercent.toFixed(1)}% max
        </div>
      </div>
      <div className="text-right text-xs tabular-nums">
        <div className="text-[var(--ink)]">
          {mine ? formatDuration(mine.seconds) : "—"}
          {rank >= 0 ? ` · #${rank + 1} of ${climb.attempts.length}` : ""}
        </div>
        <div className="text-[var(--ink-2)]">
          {mine?.heartRateBpm !== undefined ? `${Math.round(mine.heartRateBpm)} bpm · ` : ""}
          {mine ? attemptPower(mine) : ""}
        </div>
        {best && mine && best.activityId !== mine.activityId ? (
          <div className="text-[var(--ink-2)]">best here {formatDuration(best.seconds)}</div>
        ) : null}
      </div>
    </div>
  );
}

/** B · Every attempt this ride made on its route's sustained climbs, beside the rider's history there. */
export function ByClimb({ ride }: { ride: SplitsRideFixture }) {
  if (!ride.routeMatched) {
    return (
      <EmptyClimbNote text="Not matched to a route: there is no climb of the rider's own to time." />
    );
  }
  if (!ride.climbs || ride.climbs.length === 0) {
    return <EmptyClimbNote text="Matched to a route, but that route carries no sustained climb." />;
  }

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="By the climb"
    >
      <h2 className="font-medium text-sm">By the climb</h2>
      <div className="flex flex-col gap-2">
        {ride.climbs.map((climb, index) => (
          <ClimbRow
            key={climb.startMetres}
            climb={climb}
            ordinal={index + 1}
            activityId={ride.activityId}
          />
        ))}
      </div>
    </section>
  );
}

/* -------------------------------------------------------- C · By the terrain */

interface TerrainStretch {
  startMetres: number;
  endMetres: number;
  band: number;
  ascentMetres: number;
  speedKmh: number;
  movingSeconds: number;
}

function terrainStretches(ride: SplitsRideFixture): TerrainStretch[] {
  const distances = cumulativeMetres(ride.coordinates);
  return gradientRanges(ride.coordinates).map((range) => {
    const startMetres = distances[range.startIndex] ?? 0;
    const endMetres = distances[range.endIndex + 1] ?? startMetres;
    let ascentMetres = 0;
    for (let index = range.startIndex; index <= range.endIndex; index += 1) {
      const from = elevationOf(ride.coordinates[index] ?? [0, 0]) ?? 0;
      const to = elevationOf(ride.coordinates[index + 1] ?? [0, 0]) ?? 0;
      if (to > from) {
        ascentMetres += to - from;
      }
    }
    const distanceMetres = endMetres - startMetres;
    const { speedKmh } = stretchStats(ascentMetres, distanceMetres);

    return {
      startMetres,
      endMetres,
      band: range.band,
      ascentMetres,
      speedKmh,
      movingSeconds: distanceMetres > 0 ? (distanceMetres * 3.6) / speedKmh : 0,
    };
  });
}

const LANE = { width: 1000, height: 100 } as const;

/** C · Stretches cut where the ground's own gradient band changes, not every kilometre. */
export function ByTerrain({
  ride,
  activeMetres = null,
  onActiveChange,
}: {
  ride: SplitsRideFixture;
  activeMetres?: number | null;
  onActiveChange?: (metres: number | null) => void;
}) {
  const stretches = useMemo(() => terrainStretches(ride), [ride]);
  const plot = useRef<HTMLDivElement>(null);
  const onPointerMove = useCallback(
    (event: React.PointerEvent) => {
      const rect = plot.current?.getBoundingClientRect();
      if (!onActiveChange || !rect || rect.width === 0) {
        return;
      }
      const fraction = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
      onActiveChange(fraction * ride.totalMetres);
    },
    [onActiveChange, ride.totalMetres],
  );
  const onPointerLeave = useCallback(() => onActiveChange?.(null), [onActiveChange]);
  if (stretches.length === 0) {
    return null;
  }
  const fastest = Math.max(...stretches.map((stretch) => stretch.speedKmh));
  // The ride's last metre belongs to its last stretch rather than to none.
  const found =
    activeMetres === null ? -1 : stretches.findIndex((stretch) => activeMetres < stretch.endMetres);
  const active = activeMetres === null ? null : found >= 0 ? found : stretches.length - 1;
  const cursorX =
    activeMetres === null
      ? null
      : (Math.min(activeMetres, ride.totalMetres) / ride.totalMetres) * LANE.width;

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="By the terrain"
    >
      <h2 className="font-medium text-sm">By the terrain</h2>
      <div ref={plot} onPointerMove={onPointerMove} onPointerLeave={onPointerLeave}>
        <svg
          viewBox={`0 0 ${LANE.width} ${LANE.height}`}
          preserveAspectRatio="none"
          className="block h-24 w-full"
          aria-hidden="true"
        >
          {stretches.map((stretch, index) => {
            const x = (stretch.startMetres / ride.totalMetres) * LANE.width;
            const width =
              ((stretch.endMetres - stretch.startMetres) / ride.totalMetres) * LANE.width;
            const bar = fastest > 0 ? (stretch.speedKmh / fastest) * LANE.height : 0;

            return (
              <rect
                key={stretch.startMetres}
                x={x + 1}
                y={LANE.height - bar}
                width={Math.max(width - 2, 0)}
                height={bar}
                rx={1}
                fill={`var(--grade-${stretch.band})`}
                opacity={active === null || active === index ? 1 : 0.45}
              />
            );
          })}
          {cursorX !== null ? (
            <line
              x1={cursorX}
              y1={0}
              x2={cursorX}
              y2={LANE.height}
              stroke="var(--ink)"
              strokeWidth={2}
            />
          ) : null}
        </svg>
      </div>
      <p className="text-[var(--ink-2)] text-xs tabular-nums">
        {active === null || active < 0
          ? `${stretches.length} stretches, cut where the grade changed.`
          : terrainReadout(stretches[active])}
      </p>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="text-[var(--ink-2)] text-xs">
              <th className="pb-1 text-left font-normal">Terrain</th>
              <th className="pb-1 text-right font-normal">Distance</th>
              <th className="pb-1 text-right font-normal">Time</th>
              <th className="pb-1 text-right font-normal">Ascent</th>
            </tr>
          </thead>
          <tbody>
            {stretches.map((stretch, index) => (
              <tr
                key={stretch.startMetres}
                className={`border-[var(--rule)] border-t ${active === index ? "text-[var(--ink)]" : "text-[var(--ink-2)]"}`}
              >
                <td className="py-1">{GRADIENT_BANDS[stretch.band]?.label}</td>
                <td className="py-1 text-right tabular-nums">
                  {formatKilometres(stretch.endMetres - stretch.startMetres)}
                </td>
                <td className="py-1 text-right tabular-nums">
                  {formatDuration(stretch.movingSeconds)}
                </td>
                <td className="py-1 text-right tabular-nums">
                  {formatAscent(stretch.ascentMetres)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function terrainReadout(stretch: TerrainStretch | undefined): string {
  if (!stretch) {
    return "";
  }
  const parts = [
    formatKilometres(stretch.endMetres),
    `${GRADIENT_BANDS[stretch.band]?.label} band`,
    `${stretch.speedKmh.toFixed(1)} km/h`,
  ];
  if (stretch.ascentMetres > 0) {
    parts.push(formatAscent(stretch.ascentMetres));
  }
  return parts.join(" · ");
}

/* ------------------------------------------------------------- D · Pacing */

function partsOf<T>(items: T[], count: number): T[][] {
  const size = Math.ceil(items.length / count);
  return Array.from({ length: count }, (_, index) =>
    items.slice(index * size, (index + 1) * size),
  ).filter((part) => part.length > 0);
}

function partStats(part: ActivitySplit[]) {
  const distanceMetres = part.reduce((sum, split) => sum + split.distanceMetres, 0);
  const movingSeconds = part.reduce((sum, split) => sum + split.movingSeconds, 0);
  const speedKmh = movingSeconds > 0 ? (distanceMetres / movingSeconds) * 3.6 : 0;
  const heartRateBpm =
    part.reduce((sum, split) => sum + (split.heartRateBpm ?? 0) * split.movingSeconds, 0) /
    (movingSeconds || 1);
  const powerWatts =
    part.reduce((sum, split) => sum + (split.powerWatts ?? 0) * split.movingSeconds, 0) /
    (movingSeconds || 1);

  return { speedKmh, heartRateBpm, powerWatts };
}

const PART_LABELS = ["First third", "Middle third", "Final third"];

/** D · The ride in three parts, and how much the second half cost against the first. */
export function Pacing({ ride }: { ride: SplitsRideFixture }) {
  const parts = useMemo(() => partsOf(ride.splits, 3).map(partStats), [ride.splits]);

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Pacing"
    >
      <h2 className="font-medium text-sm">Pacing</h2>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="text-[var(--ink-2)] text-xs">
            <th className="pb-1 text-left font-normal">Part</th>
            <th className="pb-1 text-right font-normal">Speed</th>
            <th className="pb-1 text-right font-normal">Heart rate</th>
            <th className="pb-1 text-right font-normal">Power</th>
          </tr>
        </thead>
        <tbody>
          {parts.map((part, index) => (
            <tr key={PART_LABELS[index]} className="border-[var(--rule)] border-t">
              <td className="py-1">{PART_LABELS[index]}</td>
              <td className="py-1 text-right tabular-nums">{formatSpeed(part.speedKmh)}</td>
              <td className="py-1 text-right tabular-nums">{Math.round(part.heartRateBpm)} bpm</td>
              <td className="py-1 text-right tabular-nums">{Math.round(part.powerWatts)} W</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="text-[var(--ink-2)] text-xs tabular-nums">
        {ride.metrics.decouplingPercent !== undefined
          ? `${ride.metrics.decouplingPercent.toFixed(1)}% decoupling, first half to second.`
          : "No decoupling figure for this ride."}
      </p>
    </section>
  );
}
