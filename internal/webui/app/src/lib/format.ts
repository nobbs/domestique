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
