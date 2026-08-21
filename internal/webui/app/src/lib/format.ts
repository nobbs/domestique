/** Presentation helpers. Units are metric, matching the stored values. */

export function formatDistance(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }
  if (metres < 1000) {
    return `${Math.round(metres)} m`;
  }
  const kilometres = metres / 1000;
  return `${kilometres.toFixed(kilometres < 100 ? 1 : 0)} km`;
}

export function formatCount(value: number, singular: string, plural = `${singular}s`): string {
  return `${value.toLocaleString()} ${value === 1 ? singular : plural}`;
}

/** Total climbing. Zero means the source had no usable elevation profile. */
export function formatAscent(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }

  return `${Math.round(metres).toLocaleString()} m`;
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
