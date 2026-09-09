/**
 * The four ride shapes the effort panel's figures can take, as fixtures for
 * the panel-layout spike. Static and synthetic — no personal ride data.
 */

import type { Activity, ActivityMetrics } from "../../../api/types";

const ZONE_BOUNDS = [128, 145, 158, 170];

function ride(distanceMetres: number, movingSeconds: number, metrics: ActivityMetrics): Activity {
  return {
    id: 1,
    startedAt: "2026-09-06T10:00:00+02:00",
    distanceMetres,
    movingSeconds,
    elapsedSeconds: movingSeconds + 240,
    ascentMetres: 640,
    typeId: 1,
    locationId: 1,
    metrics,
  };
}

/** A power-meter ride: every sensor figure and load scale a meter allows. */
export const POWER_RIDE = ride(58_000, 6_840, {
  zoneBoundsBpm: ZONE_BOUNDS,
  zoneSeconds: [820, 2_460, 2_050, 1_230, 280],
  averageHeartRateBpm: 148,
  maxHeartRateBpm: 178,
  averageCadenceRpm: 82,
  averagePowerWatts: 214,
  normalizedPowerWatts: 231,
  intensityFactor: 0.84,
  powerTss: 96,
  heartRateTss: 91,
  trimp: 168,
  decouplingPercent: 4.2,
  maxSpeedKmh: 61.3,
});

/** A strap-only ride: no meter, so power is an estimate with its diagnostics. */
export const STRAP_RIDE = ride(42_000, 5_400, {
  zoneBoundsBpm: ZONE_BOUNDS,
  zoneSeconds: [640, 1_980, 1_640, 980, 160],
  averageHeartRateBpm: 141,
  maxHeartRateBpm: 172,
  averageCadenceRpm: 79,
  estimatedPowerWatts: 187,
  estimateQuality: {
    autocorrelation: 0.91,
    meanAbsDeltaWattsPerSecond: 8.4,
    clipBiasWatts: 2.1,
  },
  heartRateTss: 78,
  trimp: 141,
  maxSpeedKmh: 54.7,
});

/** A bare ride: speed only, no strap or meter, no zones. */
export const BARE_RIDE = ride(31_000, 4_320, {
  maxSpeedKmh: 48.2,
});

/** The strap ride again, but with no time-in-zone at all (a profile with no threshold). */
const {
  zoneBoundsBpm: _zoneBoundsBpm,
  zoneSeconds: _zoneSeconds,
  ...strapWithoutZones
} = STRAP_RIDE.metrics ?? {};
export const STRAP_NO_ZONES_RIDE = ride(42_000, 5_400, strapWithoutZones);
