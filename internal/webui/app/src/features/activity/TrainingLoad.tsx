/**
 * How hard one ride was, in two boxes: its time in heart-rate zones, and beside
 * it what its sensors averaged and the load it came to on every scale allowed.
 *
 * The load scales are shown together rather than reconciled — they answer
 * different questions and neither converts to the other — and each is named,
 * so a number is never a bare figure the reader has to guess the meaning of.
 * Anything the ride's sensors or the rider's profile did not allow is left out
 * rather than shown as a zero. The figures are tiles in rows — the sensors,
 * then power, load, physiology — each tinted in the colour its series is
 * drawn in, so the colour does the grouping and no heading has to.
 */

import {
  IconActivity,
  IconBolt,
  IconFlame,
  IconGauge,
  IconHeart,
  IconInfoCircle,
  IconRotate,
} from "@tabler/icons-react";
import type { ComponentType } from "react";
import type { Activity, ActivityMetrics } from "../../api/types";
import { PanelHeading } from "../../components/PanelHeading";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../components/ui/tooltip";
import { formatCoverage } from "../../lib/format";
import { HeartRateZones } from "./HeartRateZones";

type Mark = ComponentType<{ size?: number; stroke?: number; "aria-hidden"?: "true" }>;

/** One figure: what it is called, the scale it is on, and the colour its series wears. */
export interface Scale {
  label: string;
  /** The unit or short scale read beside the value. */
  scale: string;
  /** What the figure is and where it came from, behind an info mark after its name. */
  detail?: string;
  value: number | undefined;
  decimals?: number;
  /** The ride's peak of the same series, read in its own column beside the average. */
  max?: number | undefined;
  /** The series' own share of the ride's moving time it held a reading for. Shown only below 100%: a full series has nothing to add. */
  coverage?: number | undefined;
  colour: string;
  icon: Mark;
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

const SPEED = "var(--series-speed)";
const HEART = "var(--series-heart-rate)";
const CADENCE = "var(--series-cadence)";
const POWER = "var(--series-power)";
const LOAD = "var(--alert)";
const PHYSIOLOGY = "var(--hold)";

/** A sensor's average with its peak folded in, or the peak alone where nothing was averaged. */
function paired(
  label: string,
  unit: string,
  average: number | undefined,
  max: number | undefined,
  colour: string,
  icon: Mark,
  extra: Pick<Scale, "decimals" | "coverage"> = {},
): Scale[] {
  if (average !== undefined) {
    return [{ label, scale: unit, value: average, max, colour, icon, ...extra }];
  }
  if (max !== undefined) {
    return [
      { label: `Max ${label.toLowerCase()}`, scale: unit, value: max, colour, icon, ...extra },
    ];
  }

  return [];
}

export interface Groups {
  sensors: Scale[];
  power: Scale[];
  load: Scale[];
  physiology: Scale[];
}

export function buildGroups(ride: Activity, metrics: ActivityMetrics | undefined): Groups {
  const sensors: Scale[] = [
    ...paired(
      "Speed",
      "km/h",
      averageSpeedKmh(ride, metrics),
      metrics?.maxSpeedKmh,
      SPEED,
      IconGauge,
      {
        decimals: 1,
      },
    ),
    ...paired(
      "Heart rate",
      "bpm",
      metrics?.averageHeartRateBpm,
      metrics?.maxHeartRateBpm,
      HEART,
      IconHeart,
      { coverage: metrics?.heartRateCoverage },
    ),
    ...paired(
      "Cadence",
      "rpm",
      metrics?.averageCadenceRpm,
      metrics?.maxCadenceRpm,
      CADENCE,
      IconRotate,
    ),
  ];

  // Never beside a measured average: the service serves one or the other, and
  // the label carries the estimate's provenance so it cannot read as a reading.
  const average: Scale | undefined =
    metrics?.averagePowerWatts !== undefined
      ? {
          label: "Power",
          scale: "watts average",
          value: metrics.averagePowerWatts,
          coverage: metrics.powerCoverage,
          colour: POWER,
          icon: IconBolt,
        }
      : metrics?.estimatedPowerWatts !== undefined
        ? {
            label: "Estimated power",
            scale: "watts",
            detail:
              metrics.estimatedPedallingShare === undefined
                ? "Worked out from the track"
                : `While pedalling, ${Math.round(metrics.estimatedPedallingShare * 100)}% of its estimated samples`,
            value: metrics.estimatedPowerWatts,
            colour: POWER,
            icon: IconBolt,
          }
        : undefined;

  const power: Scale[] = [
    ...(average ? [average] : []),
    // No coverage mark: this is the device's own session maximum, never a
    // figure our own power series produces, so our series' coverage is not
    // a fact about it -- unlike the average beside it, which the series
    // itself yields whenever the session declares none.
    {
      label: "Max power",
      scale: "watts",
      value: metrics?.maxPowerWatts,
      colour: POWER,
      icon: IconBolt,
    },
    {
      label: "Normalized power",
      scale: "watts",
      value: metrics?.normalizedPowerWatts,
      coverage: metrics?.powerCoverage,
      colour: POWER,
      icon: IconBolt,
    },
    {
      label: "Threshold power",
      scale: "watts",
      detail: "Set on the device",
      value: metrics?.thresholdPowerWatts,
      colour: POWER,
      icon: IconBolt,
    },
  ].filter((figure) => figure.value !== undefined);

  const load: Scale[] = [
    {
      label: "Intensity",
      scale: "of threshold",
      value: metrics?.intensityFactor,
      decimals: 2,
      coverage: metrics?.powerCoverage,
      colour: LOAD,
      icon: IconFlame,
    },
    {
      label: "TSS",
      scale: "power",
      value: metrics?.powerTss,
      coverage: metrics?.powerCoverage,
      colour: LOAD,
      icon: IconFlame,
    },
    {
      label: "hrTSS",
      scale: "heart rate",
      value: metrics?.heartRateTss,
      coverage: metrics?.heartRateCoverage,
      colour: LOAD,
      icon: IconFlame,
    },
    {
      label: "TRIMP",
      scale: "Banister",
      value: metrics?.trimp,
      coverage: metrics?.heartRateCoverage,
      colour: LOAD,
      icon: IconFlame,
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
      scale: "%",
      detail: "Share of the power-to-heart-rate ratio lost over the second half",
      value: metrics?.decouplingPercent,
      decimals: 1,
      coverage: physiologyCoverage,
      colour: PHYSIOLOGY,
      icon: IconActivity,
    },
    {
      label: "Heat drift",
      scale: "bpm",
      ...(metrics?.heatDrift === undefined
        ? {}
        : {
            detail: `Held in the endurance band at ${Math.round(metrics.heatDrift.temperatureCelsius)} °C`,
          }),
      value: metrics?.heatDrift?.heartRateBpm,
      coverage: physiologyCoverage,
      colour: PHYSIOLOGY,
      icon: IconActivity,
    },
  ].filter((figure) => figure.value !== undefined);

  return { sensors, power, load, physiology };
}

export interface Group {
  title: string;
  figures: Scale[];
}

/** The rows, in order, leaving out any the ride has nothing for. */
export function groupedSections(groups: Groups): Group[] {
  return [
    { title: "Sensors", figures: groups.sensors },
    { title: "Power", figures: groups.power },
    { title: "Load", figures: groups.load },
    { title: "Physiology", figures: groups.physiology },
  ].filter((group) => group.figures.length > 0);
}

const BOX = "flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]";

/**
 * One hairline table a group, its name muted above it. A group whose figures
 * carry a peak keeps a second column for it; any other reads a single column.
 */
function GroupList({ groups }: { groups: Group[] }) {
  return (
    <div className="flex flex-col gap-3">
      {groups.map((group) => {
        const figures = group.figures.filter((figure) => figure.value !== undefined);
        const paired = figures.some((figure) => figure.max !== undefined);
        return (
          <table key={group.title} className="w-full text-sm tabular-nums">
            <thead>
              <tr>
                <th
                  colSpan={paired ? 3 : 2}
                  className="pb-1 text-left font-normal text-[var(--ink-2)] text-xs"
                >
                  {group.title}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--rule)]">
              {figures.map((figure) => (
                <FigureRow key={figure.label} figure={figure} paired={paired} />
              ))}
            </tbody>
          </table>
        );
      })}
    </div>
  );
}

function FigureRow({ figure, paired }: { figure: Scale; paired: boolean }) {
  const Icon = figure.icon;
  const coverageNote = formatCoverage(figure.coverage);
  const decimals = figure.decimals ?? 0;

  return (
    <tr>
      <th scope="row" className="py-1.5 pr-2 text-left font-normal">
        <span className="flex items-center gap-2">
          <span style={{ color: figure.colour }}>
            <Icon size={15} stroke={1.8} aria-hidden="true" />
          </span>
          <span>{figure.label}</span>
          {figure.detail ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    aria-label={figure.detail}
                    className="-m-1 rounded p-1 text-[var(--ink-2)] hover:text-[var(--ink)]"
                  />
                }
              >
                <IconInfoCircle size={13} stroke={1.8} aria-hidden="true" />
              </TooltipTrigger>
              <TooltipContent>{figure.detail}</TooltipContent>
            </Tooltip>
          ) : null}
        </span>
        {coverageNote ? (
          <span className="block pl-[23px] text-[var(--ink-2)] text-xs opacity-70">
            {coverageNote}
          </span>
        ) : null}
      </th>
      <td className="py-1.5 text-right font-semibold">
        {figure.value?.toFixed(decimals)}
        <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">{figure.scale}</span>
      </td>
      {paired ? (
        <td className="w-16 whitespace-nowrap py-1.5 text-right text-[var(--ink-2)] text-xs">
          {figure.max === undefined ? null : `max ${figure.max.toFixed(decimals)}`}
        </td>
      ) : null}
    </tr>
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
      className={
        zones && sections.length > 0
          ? "grid items-start gap-4 md:grid-cols-2 lg:grid-cols-1"
          : "grid"
      }
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
          <PanelHeading icon={<IconActivity size={18} stroke={1.8} />} title={sections[0]?.title} />
          <GroupList groups={sections} />
        </section>
      ) : null}
    </div>
  );
}
