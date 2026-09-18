/**
 * A ride's time in heart-rate zones, two ways: as shares of a ring, and as the
 * spread of every whole heart rate it held under the zones' edges.
 *
 * The ring and its table come with the ride; the spread is fetched only once
 * the rider switches to it. Both keep the table in view, and pointing at a
 * zone anywhere — a ring segment, a row, a bar — lights that zone in the rest.
 */

import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { activityHeartRateDistributionQuery } from "../../api/queries";
import { ApiError } from "../../api/request";
import type { ActivityHeartRateDistribution } from "../../api/types";
import { DonutChart } from "../../components/chart/DonutChart";
import { HistogramChart } from "../../components/chart/HistogramChart";
import { type LegendRow, LegendTable } from "../../components/chart/LegendTable";
import { SwitchableView, type View } from "../../components/SwitchableView";
import { Skeleton } from "../../components/ui/skeleton";
import { formatCoverage, formatDuration, formatShare } from "../../lib/format";

/** The five zones, easiest first, as a rider reading a training app knows them. */
export const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

/** Easiest to hardest on the severity ramp the gradient bands wear. */
export function zoneColour(zone: number): string {
  return `var(--grade-${zone})`;
}

/**
 * The first whole heart rate of each zone above the easiest. A bound cut from a
 * percentage lands between two beats, and a sample below it is still the
 * easier zone, so each edge takes the ceiling rather than the nearer beat.
 */
function zoneEdges(bounds: number[]): number[] {
  return bounds.map((bound) => Math.ceil(bound));
}

/**
 * The heart rates each zone covers, easiest first. Open at both ends — the
 * easiest zone has nothing below it and the hardest nothing above — so neither
 * is given a limit the profile never said.
 */
function zoneRanges(bounds: number[]): string[] {
  const ranges: string[] = [];
  let low: number | undefined;
  for (const edge of zoneEdges(bounds)) {
    ranges.push(low === undefined ? `below ${edge} bpm` : `${low}–${edge - 1} bpm`);
    low = edge;
  }
  if (low !== undefined) {
    ranges.push(`${low} bpm and up`);
  }

  return ranges;
}

type ZoneView = "zones" | "distribution";

export interface HeartRateZonesProps {
  rideId: string;
  zoneSeconds: number[];
  zoneBounds: number[] | undefined;
  deviceZoneSeconds: number[] | undefined;
  coverage: number | undefined;
}

export function HeartRateZones({
  rideId,
  zoneSeconds,
  zoneBounds,
  deviceZoneSeconds,
  coverage,
}: HeartRateZonesProps) {
  const [active, setActive] = useState<number | null>(null);
  const total = zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);
  if (total <= 0) {
    return null;
  }
  const ranges = zoneBounds ? zoneRanges(zoneBounds) : [];
  const coverageNote = formatCoverage(coverage);
  const rows: LegendRow<number>[] = zoneSeconds.map((seconds, zone) => ({
    key: zone,
    colour: zoneColour(zone),
    label: ZONE_NAMES[zone] ?? "",
    detail: ranges[zone],
    value: formatDuration(seconds),
    share: formatShare(seconds, total),
  }));
  const table = <LegendTable rows={rows} active={active} onActive={setActive} />;
  const footnotes = coverageNote ? (
    <p className="text-[10px] text-[var(--ink-2)] opacity-70">{coverageNote}</p>
  ) : null;

  const views: View<ZoneView>[] = [
    {
      value: "zones",
      label: "Zones",
      content: () => (
        <>
          <div className="flex justify-center">
            <DonutChart
              segments={zoneSeconds.map((seconds, zone) => ({
                key: zone,
                value: seconds,
                colour: zoneColour(zone),
              }))}
              active={active}
              onActive={setActive}
            >
              <span className="font-semibold text-lg tabular-nums">
                {formatDuration(active === null ? total : zoneSeconds[active])}
              </span>
              <span className="text-[var(--ink-2)] text-xs">
                {active === null ? "in zones" : ZONE_NAMES[active]}
              </span>
            </DonutChart>
          </div>
          {table}
          {deviceZoneSeconds && deviceZoneSeconds.length > 0 ? (
            // The profile's zones above are the default; this is only a caption
            // naming the head unit's own cut of the same ride, for comparison.
            <p className="text-[var(--ink-2)] text-xs opacity-70">
              Device zones:{" "}
              {deviceZoneSeconds.map((seconds) => formatDuration(seconds)).join(" · ")}
            </p>
          ) : null}
          {footnotes}
        </>
      ),
    },
  ];
  // The spread is coloured and marked by the zones' own edges, so a row
  // derived without its bounds has nothing to draw it against.
  if (zoneBounds && zoneBounds.length > 0) {
    views.push({
      value: "distribution",
      label: "Distribution",
      content: () => (
        <>
          <Distribution
            rideId={rideId}
            edges={zoneEdges(zoneBounds)}
            active={active}
            onActive={setActive}
          />
          {table}
          {footnotes}
        </>
      ),
    });
  }

  return (
    <SwitchableView
      label="Heart-rate view"
      heading={<h2 className="font-semibold text-base">Heart rate</h2>}
      views={views}
      // A row that was pointed at when its view went away never saw the pointer leave.
      onValueChange={() => setActive(null)}
    />
  );
}

function Distribution({
  rideId,
  edges,
  active,
  onActive,
}: {
  rideId: string;
  edges: number[];
  active: number | null;
  onActive: (zone: number | null) => void;
}) {
  const { data, error, isPending } = useQuery(activityHeartRateDistributionQuery(rideId));
  if (isPending) {
    return <Skeleton className="h-36 w-full" />;
  }
  if (error || !data) {
    return (
      <p className="flex h-36 items-center justify-center text-[var(--ink-2)] text-xs">
        {error instanceof ApiError && error.isNotFound
          ? "No heart rates are stored for this ride."
          : "The service did not say how long the ride held each heart rate."}
      </p>
    );
  }

  return (
    <DistributionChart distribution={data} edges={edges} active={active} onActive={onActive} />
  );
}

/** How many whole heart rates one bar of the distribution holds. */
const BUCKET_BPM = 5;

export interface BeatBucket {
  /** The first and last whole heart rate the bucket holds, inclusive. */
  fromBpm: number;
  toBpm: number;
  seconds: number;
  zone: number;
}

/**
 * Groups a distribution into five-beat buckets on round numbers, cut at either
 * end and at every zone edge, so each bar is coloured by one zone.
 */
export function bucketBeats(
  distribution: ActivityHeartRateDistribution,
  edges: number[],
): BeatBucket[] {
  const { fromBpm, seconds } = distribution;
  const buckets: BeatBucket[] = [];
  seconds.forEach((held, index) => {
    const bpm = fromBpm + index;
    const zone = edges.filter((edge) => bpm >= edge).length;
    const last = buckets.at(-1);
    if (
      last &&
      last.zone === zone &&
      Math.floor(last.fromBpm / BUCKET_BPM) === Math.floor(bpm / BUCKET_BPM)
    ) {
      last.toBpm = bpm;
      last.seconds += held;
    } else {
      buckets.push({ fromBpm: bpm, toBpm: bpm, seconds: held, zone });
    }
  });

  return buckets;
}

function DistributionChart({
  distribution,
  edges,
  active,
  onActive,
}: {
  distribution: ActivityHeartRateDistribution;
  edges: number[];
  active: number | null;
  onActive: (zone: number | null) => void;
}) {
  const { fromBpm, seconds } = distribution;
  const total = seconds.reduce((sum, held) => sum + held, 0);
  const buckets = bucketBeats(distribution, edges);
  const readout = (index: number): ReactNode => {
    const bucket = buckets[index];
    if (!bucket) {
      return null;
    }

    return (
      <>
        <span className="font-medium">
          {bucket.fromBpm === bucket.toBpm ? bucket.fromBpm : `${bucket.fromBpm}–${bucket.toBpm}`}{" "}
          bpm
        </span>
        <span className="text-[var(--ink-2)]">
          {formatDuration(bucket.seconds)} · {formatShare(bucket.seconds, total)}
        </span>
      </>
    );
  };

  return (
    <HistogramChart
      label="Time at each heart rate"
      bars={buckets.map((bucket) => ({
        value: bucket.seconds,
        span: bucket.toBpm - bucket.fromBpm + 1,
        colour: zoneColour(bucket.zone),
        group: bucket.zone,
      }))}
      markers={edges
        .map((edge) => ({ edge: edge - fromBpm, label: String(edge) }))
        .filter((marker) => marker.edge > 0 && marker.edge < seconds.length)}
      unit="bpm"
      activeGroup={active}
      onActiveGroup={onActive}
      readout={readout}
    />
  );
}
