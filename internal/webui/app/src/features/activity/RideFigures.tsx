/**
 * The plain figures a rider looks at first, beside the training load rather
 * than inside it: what the ride's own sensors averaged, and how fast it was.
 *
 * Every one but the speed is worked out from the stored samples and served on
 * the ride's metrics, so a ride carrying no strap or no meter simply has none
 * of that figure. The speed is the ride's own totals divided, which is why it
 * is here even for a ride whose recorded file was never readable.
 */

import type { Activity } from "../../api/types";
import { Figure, type Scale } from "./TrainingLoad";

/**
 * The ride's average speed in kilometres per hour, from the summary totals.
 *
 * A ride whose moving time is nought — one still being recorded, or one whose
 * summary carried none — has no speed rather than an infinite one.
 */
function averageSpeedKmh(ride: Activity): number | undefined {
  if (!Number.isFinite(ride.distanceMetres) || !(ride.movingSeconds > 0)) {
    return undefined;
  }

  return (ride.distanceMetres / ride.movingSeconds) * 3.6;
}

export function RideFigures({ ride }: { ride: Activity | undefined }) {
  if (!ride) {
    return null;
  }
  const metrics = ride.metrics;
  const figures: Scale[] = [
    { label: "Speed", scale: "km/h average", value: averageSpeedKmh(ride), decimals: 1 },
    { label: "Heart rate", scale: "bpm average", value: metrics?.averageHeartRateBpm },
    { label: "Max heart rate", scale: "bpm", value: metrics?.maxHeartRateBpm },
    { label: "Cadence", scale: "rpm average", value: metrics?.averageCadenceRpm },
    { label: "Power", scale: "watts average", value: metrics?.averagePowerWatts },
  ];
  const shown = figures.filter((figure) => figure.value !== undefined);
  if (shown.length === 0) {
    return null;
  }

  return (
    <section
      className="flex flex-wrap gap-x-8 gap-y-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Ride averages"
    >
      {shown.map((figure) => (
        <Figure key={figure.label} {...figure} />
      ))}
    </section>
  );
}
