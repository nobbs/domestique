/**
 * How hard one ride was, in two boxes: its time in heart-rate zones, and beside
 * it what its sensors averaged and the load it came to on every scale allowed.
 *
 * The load scales are shown together rather than reconciled — they answer
 * different questions and neither converts to the other — and each is named,
 * so a number is never a bare figure the reader has to guess the meaning of.
 * Anything the ride's sensors or the rider's profile did not allow is left out
 * rather than shown as a zero. The figures group under small headings — the
 * box's own Sensors, then Power, Load, Physiology — so a reader can skim past what a ride's
 * shape does not carry rather than meet an eighteen-tile grid every time.
 */

import type { ReactNode } from "react";
import type { Activity, ActivityMetrics } from "../../api/types";
import { Separator } from "../../components/ui/separator";
import { formatCoverage } from "../../lib/format";
import { HeartRateZones } from "./HeartRateZones";

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
  const coverageNote = formatCoverage(coverage);

  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[var(--ink-2)] text-xs">{label}</span>
      <span className="font-semibold text-lg tabular-nums">{value.toFixed(decimals)}</span>
      <span className="text-[var(--ink-2)] text-xs">{scale}</span>
      {coverageNote ? (
        <span className="text-[10px] text-[var(--ink-2)] opacity-70">{coverageNote}</span>
      ) : null}
    </div>
  );
}

/**
 * The worse of two series' coverage, for a figure built from both — decoupling
 * and heat drift each need measured power and heart rate together, so either
 * one falling short makes the figure no more trustworthy than its weaker half.
 */
function combinedCoverage(a: number | undefined, b: number | undefined): number | undefined {
  if (a === undefined) {
    return b;
  }
  if (b === undefined) {
    return a;
  }

  return Math.min(a, b);
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
    // No coverage mark: this is the device's own session maximum, never a
    // figure our own power series produces, so our series' coverage is not
    // a fact about it -- unlike the average beside it, which the series
    // itself yields whenever the session declares none.
    { label: "Max power", scale: "watts", value: metrics?.maxPowerWatts },
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
  //
  // Both figures are built from measured power and heart rate; the worse of
  // the two series' own coverage marks either -- fully describing decoupling,
  // which needs no other series. Heat drift also needs a temperature reading
  // this service tracks no coverage share for, so the service withholds it
  // below the same threshold that withholds the load figures above
  // (docs/specs/service.md) rather than mark it with a share that would
  // understate what the untracked series could be missing; heat drift's own
  // mark below only ever describes the heart-rate/power share of a ride
  // that already cleared that floor.
  const physiologyCoverage = combinedCoverage(metrics?.heartRateCoverage, metrics?.powerCoverage);
  const physiology: Scale[] = [
    {
      label: "Decoupling",
      scale: "% of ratio lost over the second half",
      value: metrics?.decouplingPercent,
      decimals: 1,
      coverage: physiologyCoverage,
    },
    {
      label: "Heat drift",
      scale:
        metrics?.heatDrift === undefined
          ? ""
          : `bpm in the endurance band at ${Math.round(metrics.heatDrift.temperatureCelsius)} °C`,
      value: metrics?.heatDrift?.heartRateBpm,
      coverage: physiologyCoverage,
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

const BOX = "flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5";

/** The box is titled by its first group, so that group needs no heading of its own. */
function GroupList({ groups }: { groups: Group[] }) {
  return (
    <div className="flex flex-col gap-4">
      {groups.map((group, index) => (
        <div key={group.title} className="flex flex-col gap-2">
          {index > 0 ? <Separator /> : null}
          {index === 0 ? null : (
            <h3 className="text-[10px] text-[var(--ink-2)] font-semibold uppercase tracking-[0.08em]">
              {group.title}
            </h3>
          )}
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
    <div
      className={zones && sections.length > 0 ? "grid items-start gap-4 md:grid-cols-2" : "grid"}
    >
      {zones ? (
        <section className={BOX} aria-label="Heart rate">
          <HeartRateZones
            rideId={ride.id}
            zoneSeconds={zones}
            zoneBounds={metrics?.zoneBoundsBpm}
            deviceZoneSeconds={metrics?.deviceZoneSeconds}
            coverage={metrics?.heartRateCoverage}
          />
        </section>
      ) : null}
      {sections.length > 0 ? (
        <section className={BOX} aria-label={sections[0]?.title}>
          <h2 className="font-medium text-sm">{sections[0]?.title}</h2>
          <GroupList groups={sections} />
        </section>
      ) : null}
    </div>
  );
}
