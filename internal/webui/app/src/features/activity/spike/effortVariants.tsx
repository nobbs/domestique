/**
 * Four ways the effort panel could weight and divide its up-to-eighteen
 * figures. Storybook only: nothing here is imported by the application. Each
 * variant takes one position on what belongs beside the zone bar, what can
 * wait behind a click, and what an estimate's diagnostics are worth showing
 * as — see the note on each variant in `EffortSpike.stories.tsx`.
 */

import type { ReactNode } from "react";
import type { Activity } from "../../../api/types";
import { Badge } from "../../../components/ui/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../../components/ui/collapsible";
import { Separator } from "../../../components/ui/separator";
import { formatDuration } from "../../../lib/format";
import { Figure, type Scale } from "../TrainingLoad";
import { BARE_RIDE, POWER_RIDE, STRAP_NO_ZONES_RIDE, STRAP_RIDE } from "./effortData";

/* ---------------------------------------------------------- zone bar, as today */

const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

function zoneColour(zone: number): string {
  return `var(--grade-${zone})`;
}

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

function ZoneStack({
  zoneSeconds,
  zoneBounds,
}: {
  zoneSeconds: number[];
  zoneBounds: number[] | undefined;
}) {
  const total = zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);
  if (total <= 0) {
    return null;
  }
  const ranges = zoneBounds ? zoneRanges(zoneBounds) : [];

  return (
    <div className="flex flex-col gap-3">
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
    </div>
  );
}

function PanelShell({ ride, children }: { ride: Activity; children: ReactNode }) {
  const metrics = ride.metrics;
  const zones = metrics?.zoneSeconds?.some((seconds) => seconds > 0) ? metrics.zoneSeconds : null;
  return (
    <section
      className="flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Effort"
    >
      <h2 className="font-medium text-sm">Effort</h2>
      <div className={zones ? "grid gap-6 md:grid-cols-2" : ""}>
        {zones ? <ZoneStack zoneSeconds={zones} zoneBounds={metrics?.zoneBoundsBpm} /> : null}
        {children}
      </div>
    </section>
  );
}

/* ------------------------------------------------------ figures, categorised */

function averageSpeedKmh(ride: Activity): number | undefined {
  if (!Number.isFinite(ride.distanceMetres) || !(ride.movingSeconds > 0)) {
    return undefined;
  }
  return (ride.distanceMetres / ride.movingSeconds) * 3.6;
}

interface Groups {
  sensors: Scale[];
  power: Scale | undefined;
  isEstimate: boolean;
  diagnostics: Scale[];
  normalizedPower: Scale | undefined;
  load: Scale[];
  physiology: Scale[];
  /** The load-scale label RideFigures already shows large on the headline, if any. */
  headlineLoadLabel: string | undefined;
}

function buildGroups(ride: Activity): Groups {
  const metrics = ride.metrics;
  const sensors: Scale[] = [
    { label: "Speed", scale: "km/h average", value: averageSpeedKmh(ride), decimals: 1 },
    { label: "Max speed", scale: "km/h", value: metrics?.maxSpeedKmh, decimals: 1 },
    { label: "Heart rate", scale: "bpm average", value: metrics?.averageHeartRateBpm },
    { label: "Max heart rate", scale: "bpm", value: metrics?.maxHeartRateBpm },
    { label: "Cadence", scale: "rpm average", value: metrics?.averageCadenceRpm },
  ].filter((figure) => figure.value !== undefined);

  const isEstimate =
    metrics?.averagePowerWatts === undefined && metrics?.estimatedPowerWatts !== undefined;
  const power: Scale | undefined =
    metrics?.averagePowerWatts !== undefined
      ? { label: "Power", scale: "watts average", value: metrics.averagePowerWatts }
      : metrics?.estimatedPowerWatts !== undefined
        ? {
            label: "Estimated power",
            scale: "watts, from the track",
            value: metrics.estimatedPowerWatts,
          }
        : undefined;

  // Fixed order — steadiness, jitter, clamp bias — so the folded caption can read positionally.
  const diagnostics: Scale[] =
    isEstimate && metrics?.estimateQuality
      ? [
          {
            label: "Estimate steadiness",
            scale: "lag-1 correlation",
            value: metrics.estimateQuality.autocorrelation,
            decimals: 2,
          },
          {
            label: "Estimate jitter",
            scale: "watts change per second",
            value: metrics.estimateQuality.meanAbsDeltaWattsPerSecond,
            decimals: 1,
          },
          {
            label: "Clamp bias",
            scale: "watts the zero clamp added",
            value: metrics.estimateQuality.clipBiasWatts,
            decimals: 1,
          },
        ]
      : [];

  const normalizedPower: Scale | undefined =
    metrics?.normalizedPowerWatts !== undefined
      ? { label: "Normalized power", scale: "watts", value: metrics.normalizedPowerWatts }
      : undefined;

  const load: Scale[] = [
    { label: "Intensity", scale: "of threshold", value: metrics?.intensityFactor, decimals: 2 },
    { label: "TSS", scale: "power", value: metrics?.powerTss },
    { label: "hrTSS", scale: "heart rate", value: metrics?.heartRateTss },
    { label: "TRIMP", scale: "Banister", value: metrics?.trimp },
  ].filter((figure) => figure.value !== undefined);

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

  const headlineLoadLabel =
    metrics?.powerTss !== undefined
      ? "TSS"
      : metrics?.heartRateTss !== undefined
        ? "hrTSS"
        : metrics?.trimp !== undefined
          ? "TRIMP"
          : undefined;

  return {
    sensors,
    power,
    isEstimate,
    diagnostics,
    normalizedPower,
    load,
    physiology,
    headlineLoadLabel,
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

/* -------------------------------------------------------------- 1 · Grouped */

interface Group {
  title: string;
  content: ReactNode;
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

/** Sections shared by the Grouped, Folded and Lean variants: the diagnostics
 * either sit as tiles under Power or are folded into the power tile's caption,
 * and Lean additionally drops the one load scale the headline already shows. */
function groupedSections(
  groups: Groups,
  options: { foldDiagnostics: boolean; dropHeadlineLoad: boolean },
): Group[] {
  const powerContent: ReactNode[] = [];
  if (groups.power) {
    if (options.foldDiagnostics && groups.isEstimate && groups.diagnostics.length === 3) {
      powerContent.push(
        <EstimatedPowerTile key="power" power={groups.power} diagnostics={groups.diagnostics} />,
      );
    } else {
      powerContent.push(<Figure key={groups.power.label} {...groups.power} />);
      for (const diagnostic of groups.diagnostics) {
        powerContent.push(<Figure key={diagnostic.label} {...diagnostic} />);
      }
    }
  }
  if (groups.normalizedPower) {
    powerContent.push(<Figure key="normalized-power" {...groups.normalizedPower} />);
  }

  const load = options.dropHeadlineLoad
    ? groups.load.filter((figure) => figure.label !== groups.headlineLoadLabel)
    : groups.load;

  const sections: Group[] = [];
  if (groups.sensors.length > 0) {
    sections.push({ title: "Sensors", content: figureGrid(groups.sensors) });
  }
  if (powerContent.length > 0) {
    sections.push({ title: "Power", content: <div className={GRID}>{powerContent}</div> });
  }
  if (load.length > 0) {
    sections.push({ title: "Load", content: figureGrid(load) });
  }
  if (groups.physiology.length > 0) {
    sections.push({ title: "Physiology", content: figureGrid(groups.physiology) });
  }
  return sections;
}

export function GroupedPanel({ ride }: { ride: Activity }) {
  const sections = groupedSections(buildGroups(ride), {
    foldDiagnostics: false,
    dropHeadlineLoad: false,
  });
  return (
    <PanelShell ride={ride}>
      <GroupList groups={sections} />
    </PanelShell>
  );
}

/* --------------------------------------------------- 2 · Primary + disclosure */

export function PrimaryDisclosurePanel({
  ride,
  defaultOpen = false,
}: {
  ride: Activity;
  defaultOpen?: boolean;
}) {
  const groups = buildGroups(ride);
  const primary: Scale[] = [...groups.sensors, ...(groups.power ? [groups.power] : [])];
  const more: Scale[] = [
    ...groups.diagnostics,
    ...(groups.normalizedPower ? [groups.normalizedPower] : []),
    ...groups.load,
    ...groups.physiology,
  ];

  return (
    <PanelShell ride={ride}>
      <div className="flex flex-col gap-3">
        {primary.length > 0 ? figureGrid(primary) : null}
        {more.length > 0 ? (
          <Collapsible defaultOpen={defaultOpen}>
            <CollapsibleTrigger className="text-left font-medium text-[var(--ink-2)] text-xs">
              More figures ({more.length})
            </CollapsibleTrigger>
            <CollapsibleContent className="pt-3">{figureGrid(more)}</CollapsibleContent>
          </Collapsible>
        ) : null}
      </div>
    </PanelShell>
  );
}

/* ------------------------------------------------------- 3 · Folded diagnostics */

/** Fixed positional order from `buildGroups`: steadiness, jitter, clamp bias. */
function foldedCaption(diagnostics: Scale[]): string {
  const [steadiness, jitter, bias] = diagnostics;
  const steadyWord =
    (steadiness?.value ?? 0) >= 0.85
      ? "steady"
      : (steadiness?.value ?? 0) >= 0.6
        ? "some drift"
        : "noisy";
  const jitterWord =
    (jitter?.value ?? 0) < 10
      ? "low jitter"
      : (jitter?.value ?? 0) < 20
        ? "moderate jitter"
        : "high jitter";
  const biasValue = bias?.value ?? 0;
  const biasWord = `${biasValue >= 0 ? "+" : ""}${biasValue.toFixed(0)} W clamp bias`;
  return `${steadyWord}, ${jitterWord}, ${biasWord}`;
}

function estimateTier(diagnostics: Scale[]): "steady" | "rough" {
  const [steadiness, jitter, bias] = diagnostics;
  const good =
    (steadiness?.value ?? 0) >= 0.85 && (jitter?.value ?? 0) < 12 && Math.abs(bias?.value ?? 0) < 5;
  return good ? "steady" : "rough";
}

function EstimatedPowerTile({ power, diagnostics }: { power: Scale; diagnostics: Scale[] }) {
  const hoverTitle = diagnostics
    .map(
      (diagnostic) =>
        `${diagnostic.label} ${diagnostic.value?.toFixed(diagnostic.decimals ?? 0)} ${diagnostic.scale}`,
    )
    .join(" · ");

  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[var(--ink-2)] text-xs">{power.label}</span>
      <div className="flex items-baseline gap-1.5">
        <span className="font-semibold text-lg tabular-nums">
          {power.value?.toFixed(power.decimals ?? 0)}
        </span>
        <Badge
          variant={estimateTier(diagnostics) === "steady" ? "secondary" : "outline"}
          className="h-4 px-1.5 text-[9px]"
        >
          {estimateTier(diagnostics)}
        </Badge>
      </div>
      <span className="text-[var(--ink-2)] text-xs">{power.scale}</span>
      <span className="text-[10px] text-[var(--ink-2)] opacity-80" title={hoverTitle}>
        {foldedCaption(diagnostics)}
      </span>
    </div>
  );
}

export function FoldedDiagnosticsPanel({ ride }: { ride: Activity }) {
  const sections = groupedSections(buildGroups(ride), {
    foldDiagnostics: true,
    dropHeadlineLoad: false,
  });
  return (
    <PanelShell ride={ride}>
      <GroupList groups={sections} />
    </PanelShell>
  );
}

/* ------------------------------------------------------------------ 4 · Lean */

export function LeanPanel({ ride }: { ride: Activity }) {
  const sections = groupedSections(buildGroups(ride), {
    foldDiagnostics: true,
    dropHeadlineLoad: true,
  });
  return (
    <PanelShell ride={ride}>
      <GroupList groups={sections} />
    </PanelShell>
  );
}

/* -------------------------------------------------------------------- shapes */

export const SHAPES: { label: string; ride: Activity }[] = [
  { label: "Power-meter ride", ride: POWER_RIDE },
  { label: "Strap-only ride, estimate", ride: STRAP_RIDE },
  { label: "Bare ride, speed only", ride: BARE_RIDE },
  { label: "Strap ride, no zones", ride: STRAP_NO_ZONES_RIDE },
];

/** Stacks one panel per ride shape in a column, each labelled, so a single
 * screenshot compares all four at once. */
export function ShapeStack({ Panel }: { Panel: (props: { ride: Activity }) => ReactNode }) {
  return (
    <div className="flex flex-col gap-6">
      {SHAPES.map(({ label, ride }) => (
        <div key={label} className="flex flex-col gap-2">
          <h3 className="text-[var(--ink-2)] text-xs font-semibold uppercase tracking-[0.1em]">
            {label}
          </h3>
          <Panel ride={ride} />
        </div>
      ))}
    </div>
  );
}
