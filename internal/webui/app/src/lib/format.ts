/**
 * Presentation helpers. The service stores, transmits and shows metric values
 * throughout, so nothing here converts — every formatter only chooses how much
 * of the figure it was handed is worth printing.
 */

import type { RouteValidation } from "../api/types";

/**
 * A tenth below 100 km, a whole number at or above it — always in kilometres,
 * no unit switch to metres.
 *
 * The tenth can itself round a figure up to 100.0: that crossing is re-checked
 * against the whole-number rule rather than left to print a decimal the rule
 * says this distance is too long for.
 */
export function formatKilometres(metres: number): string {
  if (!Number.isFinite(metres)) {
    return "—";
  }
  const kilometres = Math.max(metres, 0) / 1000;
  if (kilometres < 100) {
    const tenth = kilometres.toFixed(1);
    return Number(tenth) < 100 ? `${tenth} km` : `${Math.round(kilometres)} km`;
  }
  return `${Math.round(kilometres)} km`;
}

export function formatDistance(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }
  if (metres < 1000) {
    return `${Math.round(metres)} m`;
  }
  return formatKilometres(metres);
}

/** `14:20` or `02:20 PM`, in the reader's own zone and clock convention; `""` for a date that failed to parse. */
export function formatClock(at: Date): string {
  if (Number.isNaN(at.getTime())) {
    return "";
  }
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function formatCount(value: number, singular: string, plural = `${singular}s`): string {
  return `${value.toLocaleString()} ${value === 1 ? singular : plural}`;
}

function formatElevationTotal(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }

  return `${Math.round(metres).toLocaleString()} m`;
}

/**
 * Total ascent. Renders as a dash for zero or below — which reads as "no
 * usable elevation profile" for most routes, but is also the true value for
 * a route that only descends.
 */
export function formatAscent(metres: number): string {
  return formatElevationTotal(metres);
}

/** Total descent. Same dash convention as `formatAscent`, for the same reasons. */
export function formatDescent(metres: number): string {
  return formatElevationTotal(metres);
}

/**
 * The steepest sustained gradient. It is measured over a window rather than
 * between neighbouring points, so it reads as a climb rather than as satellite
 * noise; anything under a percent is not worth claiming as a gradient.
 */
export function formatGradient(percent: number): string {
  if (!Number.isFinite(percent) || percent < 1) {
    return "—";
  }

  return `${percent.toFixed(percent < 10 ? 1 : 0)}%`;
}

export function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return "never";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "unknown";
  }
  return parsed.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

/**
 * When the library was last read, as a card says it.
 *
 * The clock alone for a read that happened today, which is nearly every read a
 * reader will ever see: the service reads hourly, and a full date beside it is
 * three quarters punctuation. Anything older keeps the date, because "19:38" on
 * a stale library is the one case where the short form would mislead — and the
 * page has no way to say "the sync has been down for two days" other than by
 * showing the day.
 */
export function formatReadTime(value: string | undefined, now = new Date()): string {
  if (!value) {
    return "never";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "unknown";
  }
  if (parsed.toDateString() !== now.toDateString()) {
    return formatTimestamp(value);
  }

  return parsed.toLocaleTimeString(undefined, { timeStyle: "short" });
}

/**
 * A height above sea level.
 *
 * Not `formatAscent`: a climb of nought metres is a route with no usable
 * profile, but an altitude of nought metres is the coast, and a route that
 * drops below sea level is a real one.
 */
export function formatElevation(metres: number): string {
  if (!Number.isFinite(metres)) {
    return "—";
  }

  return `${Math.round(metres).toLocaleString()} m`;
}

/**
 * Air temperature at one forecast point.
 *
 * A decimal earns its place near freezing, where the digit after the point is
 * the difference between rain and ice; a reading already in double digits has
 * left that boundary far enough behind that the extra digit is only noise.
 */
export function formatTemperature(celsius: number): string {
  if (!Number.isFinite(celsius)) {
    return "—";
  }
  const decimals = Math.abs(celsius) < 10 ? 1 : 0;
  // A reading just below zero rounds to negative zero, and "-0°C" reads as a
  // fault rather than as a temperature.
  const rounded = Number(celsius.toFixed(decimals));
  const shown = Object.is(rounded, -0) ? 0 : rounded;

  return `${shown.toFixed(decimals)}°C`;
}

/**
 * Wind speed at one forecast point.
 *
 * The same reasoning as `formatTemperature`: a decimal separates a calm from
 * a light breeze, but is wasted once the reading is already a two-digit gale.
 */
export function formatWindSpeed(kmh: number): string {
  if (!Number.isFinite(kmh)) {
    return "—";
  }

  return `${kmh.toFixed(kmh < 10 ? 1 : 0)} km/h`;
}

/** Precipitation depth at one forecast point. */
export function formatPrecipitation(millimetres: number): string {
  if (!Number.isFinite(millimetres)) {
    return "—";
  }

  return `${millimetres.toFixed(1)} mm`;
}

/**
 * Derived from the server's own intervalSeconds, not a duplicated label.
 * Undefined means no fixed schedule.
 */
export function formatCadence(seconds: number | undefined): string {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) {
    return "No fixed schedule";
  }
  if (seconds === 3600) {
    return "Hourly";
  }
  if (seconds % 3600 === 0) {
    const hours = seconds / 3600;

    return `Every ${hours} hours`;
  }
  if (seconds < 60) {
    return seconds === 1 ? "Every second" : `Every ${seconds} seconds`;
  }
  const minutes = Math.round(seconds / 60);

  return minutes === 1 ? "Every minute" : `Every ${minutes} minutes`;
}

/**
 * A measured duration, exactly as long as it was. Unlike a predicted moving
 * time this is not rounded at all: seconds are floored at every step, and are
 * dropped only once there is an hour to show.
 */
export function formatDuration(seconds: number | undefined): string {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) {
    return "\u2014";
  }
  const whole = Math.floor(seconds);
  const hours = Math.floor(whole / 3600);
  const minutes = Math.floor((whole % 3600) / 60);
  const rest = whole % 60;
  if (hours > 0) {
    return minutes === 0 ? `${hours} h` : `${hours} h ${minutes} min`;
  }
  if (minutes > 0) {
    return rest === 0 ? `${minutes} min` : `${minutes} min ${rest} s`;
  }

  return `${rest} s`;
}

/**
 * A climb time, which is minutes and seconds: the shortest climb worth its own
 * row still takes longer than a minute, and none of them takes hours.
 */
export function formatClimbTime(seconds: number): string {
  // Rounded to the second before it is split, not after: a time of 59.6 s
  // rounded within the minute would read 0:60, which is not a time.
  const whole = Math.round(seconds);

  return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, "0")}`;
}

/**
 * Predicted moving time, rounded to the nearest five minutes — coarse enough
 * that it reads as an estimate rather than a promise. Absent, never zero,
 * when nothing has predicted this route's geometry.
 */
export function formatMovingTime(seconds: number | undefined): string {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) {
    return "—";
  }
  const roundedMinutes = Math.max(5, Math.round(seconds / 60 / 5) * 5);
  const hours = Math.floor(roundedMinutes / 60);
  const minutes = roundedMinutes % 60;
  if (hours === 0) {
    return `${minutes} min`;
  }

  return minutes === 0 ? `${hours} h` : `${hours} h ${minutes} min`;
}

/**
 * A coarse qualifier for a shown moving time, from the frozen profile's own
 * measured unseen-route error — mean absolute error, not the tighter bias,
 * since the reader wants "how far off does this usually run" rather than the
 * model's average direction of error. Undefined whenever the loaded profile
 * carries no measured benchmark result, which the caller reads as "show the
 * time unqualified" rather than as a missing value to report.
 */
export function formatMovingTimeUncertainty(
  validation: RouteValidation | undefined,
): string | undefined {
  if (!validation) {
    return undefined;
  }

  return `±${Math.round(validation.maePercent)}% typical`;
}

/** A ride's average speed, to a tenth: the difference between 24.6 and 25.1 km/h
 * is the whole reason two rides of one route are read side by side. */
export function formatSpeed(kmh: number | null | undefined): string {
  if (kmh === null || kmh === undefined || !Number.isFinite(kmh)) {
    return "—";
  }

  return `${kmh.toFixed(1)} km/h`;
}

/**
 * "92% sensor coverage" for a series that held less than the whole ride,
 * `undefined` for one that held all of it. Floored rather than rounded: a
 * share of 99.6% is still not the whole ride, and rounding it to "100%"
 * would print the one word this caption exists to rule out.
 *
 * A share that is a whole percent is only ever off by binary floating
 * point's own precision: 1044/3600 is exactly 29% but stores as
 * 28.999999999999996. A share genuinely a whole percent *below* an integer
 * (0.009999999999, meant to read as 0%) sits much further from it than that
 * — so only a scaled value within COVERAGE_INTEGER_EPSILON of its nearest
 * integer is treated as that integer; anything further is floored as-is.
 * Both branches are then clamped to 0-99: the server already bounds
 * `coverage` to [0, 1], but the caption itself must never print "100%" for a
 * value already told it is not the whole ride, nor a negative percentage if
 * that guarantee were ever to lapse.
 */
const COVERAGE_INTEGER_EPSILON = 1e-11;

export function formatCoverage(coverage: number | undefined): string | undefined {
  if (coverage === undefined || coverage >= 1) {
    return undefined;
  }
  const scaled = coverage * 100;
  const nearestInteger = Math.round(scaled);
  const percent =
    Math.abs(scaled - nearestInteger) < COVERAGE_INTEGER_EPSILON
      ? nearestInteger
      : Math.floor(scaled);

  return `${Math.min(Math.max(percent, 0), 99)}% sensor coverage`;
}
