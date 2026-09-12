/**
 * How hard one ride was: its time in heart-rate zones as one bar, beside what
 * its sensors averaged and the load it came to on every scale the ride allowed.
 *
 * The load scales are shown together rather than reconciled — they answer
 * different questions and neither converts to the other — and each is named,
 * so a number is never a bare figure the reader has to guess the meaning of.
 * Anything the ride's sensors or the rider's profile did not allow is left out
 * rather than shown as a zero. The figures group under small headings —
 * Sensors, Power, Load, Physiology — so a reader can skim past what a ride's
 * shape does not carry rather than meet an eighteen-tile grid every time.
 */

import type { ReactNode } from "react";
import type { Activity, ActivityMetrics } from "../../api/types";
import { Separator } from "../../components/ui/separator";
import { formatDuration } from "../../lib/format";

/** The five zones, easiest first, as a rider reading a training app knows them. */
const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

/** One figure: what it is called, and the scale it is on. */
export interface Scale {
  label: string;
  scale: string;
  value: number | undefined;
  decimals?: number;
  /** The series' own share of the ride's moving time it held a reading for. Shown only below 100%: a full series has nothing to add. */
  coverage?: number | undefined;
}

export function Figure({ label, scale, value, decimals = 0, coverage }: Scale) {
  if (value === undefined) {
    return null;
  }

  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[var(--ink-2)] text-xs">{label}</span>
      <span className="font-semibold text-lg tabular-nums">{value.toFixed(decimals)}</span>
      <span className="text-[var(--ink-2)] text-xs">{scale}</span>
      {coverage !== undefined && coverage < 1 ? (
        <span className="text-[10px] text-[var(--ink-2)] opacity-70">
          {Math.round(coverage * 100)}% sensor coverage
        </span>
      ) : null}
    </div>
  );
}

/**
 * The heart rates each zone covers, easiest first. Open at both ends — the
 * easiest zone has nothing below it and the hardest nothing above — so neither
 * is given a limit the profile never said. A bound cut from a percentage lands
 * between two beats, and a sample below it is still the easier zone, so both
 * edges take the ceiling rather than the nearer beat.
 */
function zoneRanges(bounds: number[]): string[] {
  const ranges: string[] = [];
  let low: number | undefined;
  for (const bound of bounds) {
    const edge = Math.ceil(bound);
    ranges.push(low === undefined ? `below ${edge} bpm` : `${low}–${edge - 1} bpm`);
    low = edge;
  }
  if (low !== undefined) {
    ranges.push(`${low} bpm and up`);
  }

  return ranges;
}

/** Easiest to hardest on the severity ramp the gradient bands wear. */
function zoneColour(zone: number): string {
  return `var(--grade-${zone})`;
}

/** One bar, five segments: the ride's time as a whole, each zone its share of it. */
function ZoneStack({
  zoneSeconds,
  zoneBounds,
  deviceZoneSeconds,
}: {
  zoneSeconds: number[];
  zoneBounds: number[] | undefined;
  deviceZoneSeconds: number[] | undefined;
}) {
  const total = zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);
  if (total <= 0) {
    return null;
  }
  const ranges = zoneBounds ? zoneRanges(zoneBounds) : [];

  return (
    <div className="flex flex-col gap-3">
      {/* The legend beside it says the same thing, so the bar is decoration. */}
      <div aria-hidden="true" className="flex h-3 overflow-hidden rounded-full bg-black/5">
        {zoneSeconds.map((seconds, zone) => (
          <span
            key={ZONE_NAMES[zone]}
            className="h-full"
            style={{ width: `${(seconds / total) * 100}%`, backgroundColor: zoneColour(zone) }}
          />
        ))}
      </div>
      <ul className="grid grid-cols-5 gap-2">
        {zoneSeconds.map((seconds, zone) => (
          // Zones are a fixed ordered set of five, so the name is their identity.
          <li key={ZONE_NAMES[zone]} className="flex flex-col gap-0.5">
            <span
              aria-hidden="true"
              className="h-1 rounded-full"
              style={{ backgroundColor: zoneColour(zone) }}
            />
            <span className="text-[var(--ink-2)] text-xs">{ZONE_NAMES[zone]}</span>
            {ranges[zone] ? (
              <span className="text-[10px] text-[var(--ink-2)] tabular-nums opacity-70">
                {ranges[zone]}
              </span>
            ) : null}
            <span className="text-sm tabular-nums">{formatDuration(seconds)}</span>
          </li>
        ))}
      </ul>
      {deviceZoneSeconds && deviceZoneSeconds.length > 0 ? (
        // The profile's zones above are the default; this is only a caption
        // naming the head unit's own cut of the same ride, for comparison.
        <p className="text-[var(--ink-2)] text-xs opacity-70">
          Device zones: {deviceZoneSeconds.map((seconds) => formatDuration(seconds)).join(" · ")}
        </p>
      ) : null}
    </div>
  );
}

/**
 * The ride's average speed in kilometres per hour, preferring the server's
 * own figure. Falls back to distance over moving time so it is there even for
 * a ride whose recorded file was never readable; a ride whose moving time is
 * nought has no speed rather than an infinite one.
 */
function averageSpeedKmh(ride: Activity, metrics: ActivityMetrics | undefined): number | undefined {
  if (Number.isFinite(metrics?.averageSpeedKmh)) {
    return metrics?.averageSpeedKmh;
  }
  if (!Number.isFinite(ride.distanceMetres) || !(ride.movingSeconds > 0)) {
    return undefined;
  }

  return (ride.distanceMetres / ride.movingSeconds) * 3.6;
}

interface Groups {
  sensors: Scale[];
  power: Scale | undefined;
  devicePower: Scale[];
  normalizedPower: Scale | undefined;
  load: Scale[];
  physiology: Scale[];
}

function buildGroups(ride: Activity, metrics: ActivityMetrics | undefined): Groups {
  const sensors: Scale[] = [
    { label: "Speed", scale: "km/h average", value: averageSpeedKmh(ride, metrics), decimals: 1 },
    { label: "Max speed", scale: "km/h", value: metrics?.maxSpeedKmh, decimals: 1 },
    {
      label: "Heart rate",
      scale: "bpm average",
      value: metrics?.averageHeartRateBpm,
      coverage: metrics?.heartRateCoverage,
    },
    {
      label: "Max heart rate",
      scale: "bpm",
      value: metrics?.maxHeartRateBpm,
      coverage: metrics?.heartRateCoverage,
    },
    { label: "Cadence", scale: "rpm average", value: metrics?.averageCadenceRpm },
    { label: "Max cadence", scale: "rpm", value: metrics?.maxCadenceRpm },
  ].filter((figure) => figure.value !== undefined);

  // Never beside a measured average: the service serves one or the other, and
  // the label carries the estimate's provenance so it cannot read as a reading.
  const power: Scale | undefined =
    metrics?.averagePowerWatts !== undefined
      ? {
          label: "Power",
          scale: "watts average",
          value: metrics.averagePowerWatts,
          coverage: metrics.powerCoverage,
        }
      : metrics?.estimatedPowerWatts !== undefined
        ? {
            label: "Estimated power",
            scale:
              metrics.estimatedPedallingShare === undefined
                ? "watts, from the track"
                : `watts while pedalling, ${Math.round(metrics.estimatedPedallingShare * 100)}% of its estimated samples`,
            value: metrics.estimatedPowerWatts,
          }
        : undefined;

  const devicePower: Scale[] = [
    {
      label: "Max power",
      scale: "watts",
      value: metrics?.maxPowerWatts,
      coverage: metrics?.powerCoverage,
    },
    {
      label: "Threshold power",
      scale: "watts set on the device",
      value: metrics?.thresholdPowerWatts,
    },
  ].filter((figure) => figure.value !== undefined);

  const normalizedPower: Scale | undefined =
    metrics?.normalizedPowerWatts !== undefined
      ? {
          label: "Normalized power",
          scale: "watts",
          value: metrics.normalizedPowerWatts,
          coverage: metrics.powerCoverage,
        }
      : undefined;

  const load: Scale[] = [
    {
      label: "Intensity",
      scale: "of threshold",
      value: metrics?.intensityFactor,
      decimals: 2,
      coverage: metrics?.powerCoverage,
    },
    { label: "TSS", scale: "power", value: metrics?.powerTss, coverage: metrics?.powerCoverage },
    {
      label: "hrTSS",
      scale: "heart rate",
      value: metrics?.heartRateTss,
      coverage: metrics?.heartRateCoverage,
    },
    {
      label: "TRIMP",
      scale: "Banister",
      value: metrics?.trimp,
      coverage: metrics?.heartRateCoverage,
    },
  ].filter((figure) => figure.value !== undefined);

  // Positive is the usual direction, and the scale says so: the reader is
  // told what the number measures rather than sold what it means. The pair is
  // one figure and its condition: the beats, at the degrees they were held
  // at. One ride is a point, not a trend.
  const physiology: Scale[] = [
    {
      label: "Decoupling",
      scale: "% of ratio lost over the second half",
      value: metrics?.decouplingPercent,
      decimals: 1,
    },
    {
      label: "Heat drift",
      scale:
        metrics?.heatDrift === undefined
          ? ""
          : `bpm in the endurance band at ${Math.round(metrics.heatDrift.temperatureCelsius)} °C`,
      value: metrics?.heatDrift?.heartRateBpm,
    },
  ].filter((figure) => figure.value !== undefined);

  return {
    sensors,
    power,
    devicePower,
    normalizedPower,
    load,
    physiology,
  };
}

const GRID = "grid grid-cols-3 gap-x-4 gap-y-3";

function figureGrid(figures: Scale[]): ReactNode {
  return (
    <div className={GRID}>
      {figures.map((figure) => (
        <Figure key={figure.label} {...figure} />
      ))}
    </div>
  );
}

interface Group {
  title: string;
  content: ReactNode;
}

function groupedSections(groups: Groups): Group[] {
  const powerContent: ReactNode[] = [];
  if (groups.power) {
    powerContent.push(<Figure key={groups.power.label} {...groups.power} />);
  }
  for (const figure of groups.devicePower) {
    powerContent.push(<Figure key={figure.label} {...figure} />);
  }
  if (groups.normalizedPower) {
    powerContent.push(<Figure key="normalized-power" {...groups.normalizedPower} />);
  }

  const sections: Group[] = [];
  if (groups.sensors.length > 0) {
    sections.push({ title: "Sensors", content: figureGrid(groups.sensors) });
  }
  if (powerContent.length > 0) {
    sections.push({ title: "Power", content: <div className={GRID}>{powerContent}</div> });
  }
  if (groups.load.length > 0) {
    sections.push({ title: "Load", content: figureGrid(groups.load) });
  }
  if (groups.physiology.length > 0) {
    sections.push({ title: "Physiology", content: figureGrid(groups.physiology) });
  }

  return sections;
}

function GroupList({ groups }: { groups: Group[] }) {
  return (
    <div className="flex flex-col gap-4">
      {groups.map((group, index) => (
        <div key={group.title} className="flex flex-col gap-2">
          {index > 0 ? <Separator /> : null}
          <h3 className="text-[10px] text-[var(--ink-2)] font-semibold uppercase tracking-[0.08em]">
            {group.title}
          </h3>
          {group.content}
        </div>
      ))}
    </div>
  );
}

export function TrainingLoad({ ride }: { ride: Activity | undefined }) {
  if (!ride) {
    return null;
  }
  const metrics = ride.metrics;
  const zones = metrics?.zoneSeconds?.some((seconds) => seconds > 0) ? metrics.zoneSeconds : null;
  const sections = groupedSections(buildGroups(ride, metrics));
  if (!zones && sections.length === 0) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Effort"
    >
      <h2 className="font-medium text-sm">Effort</h2>
      <div className={zones ? "grid gap-6 md:grid-cols-2" : ""}>
        {zones ? (
          <ZoneStack
            zoneSeconds={zones}
            zoneBounds={metrics?.zoneBoundsBpm}
            deviceZoneSeconds={metrics?.deviceZoneSeconds}
          />
        ) : null}
        {sections.length > 0 ? <GroupList groups={sections} /> : null}
      </div>
    </section>
  );
}
