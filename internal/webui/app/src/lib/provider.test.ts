import { describe, expect, it } from "vitest";
import { providerLabel } from "./provider";

describe("providerLabel", () => {
  it("names a provider this build knows a spelling for", () => {
    expect(providerLabel("veloplanner")).toBe("VeloPlanner");
    expect(providerLabel("komoot")).toBe("Komoot");
  });

  // A source this build has never heard of is still a source, so it is shown
  // as the service spelled it rather than hidden.
  it("falls back to the wire value for a provider it does not know", () => {
    expect(providerLabel("strava")).toBe("strava");
  });
});
