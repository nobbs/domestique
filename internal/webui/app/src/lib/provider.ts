/**
 * How a source provider is named to a reader.
 *
 * A private tool with two sources needs a name for each, not a logo: the wire
 * value is already stable and unique, so the only job here is spelling it the
 * way a reader would rather than the way the service does.
 */

/** The providers this build knows a display spelling for. */
const PROVIDER_LABELS: Record<string, string> = {
  veloplanner: "VeloPlanner",
  komoot: "Komoot",
};

/**
 * The name a reader sees for one provider.
 *
 * A provider this build has never heard of is shown as the service spelled
 * it rather than hidden: an unfamiliar source is still a source, and the
 * honest label is the wire value itself.
 */
export function providerLabel(provider: string): string {
  return PROVIDER_LABELS[provider] ?? provider;
}
