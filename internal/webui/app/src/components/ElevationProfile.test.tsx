import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import type { ProfileSample } from "../lib/profile";
import { buildProfile } from "../lib/profile";
import { ElevationProfile, steadyBands } from "./ElevationProfile";

/** Only the band matters to steadyBands; the rest is filler. */
function samplesOf(bands: number[]): ProfileSample[] {
  return bands.map((band) => ({
    distanceMetres: 0,
    elevationMetres: 0,
    longitude: 8,
    latitude: 49,
    gradientPercent: 0,
    band,
  }));
}

function climb(): Position[] {
  return Array.from(
    { length: 40 },
    (_, index): Position => [8, 49 + index * 0.001, 100 + index * 5],
  );
}

/** The chart is controlled, so exercising it needs something to hold the value. */
function Harness({ title = "Eich Rundkurs 90" }: { title?: string }) {
  const [activeIndex, setActiveIndex] = useState<number | null>(null);

  return (
    <ElevationProfile
      profile={buildProfile(climb())}
      title={title}
      activeIndex={activeIndex}
      onActiveChange={setActiveIndex}
    />
  );
}

describe("ElevationProfile", () => {
  it("describes the profile for a reader who cannot see it", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeIndex={null}
        onActiveChange={() => {}}
      />,
    );

    const figure = screen.getByRole("img", { name: /Eich Rundkurs 90/ });
    expect(figure).toHaveAccessibleName(/kilometres/);
    expect(figure).toHaveAccessibleName(/metres above sea level/);
  });

  it("exposes scrubbing as a slider so it works by keyboard", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeIndex={null}
        onActiveChange={() => {}}
      />,
    );

    const scrub = screen.getByRole("slider", { name: /Position along Eich/ });
    expect(scrub).toHaveAttribute("tabindex", "0");
    expect(scrub).toHaveAttribute("aria-valuemin", "0");
    expect(Number(scrub.getAttribute("aria-valuemax"))).toBeGreaterThan(0);
  });

  it("announces the value under the cursor when stepped by keyboard", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const scrub = screen.getByRole("slider", { name: /Position along Eich/ });
    await user.tab();
    await user.keyboard("{ArrowRight}");

    expect(scrub.getAttribute("aria-valuetext")).toMatch(/metres at .* kilometres/);
  });

  it("reports the scrubbed position outward, so the map can mark it", async () => {
    const user = userEvent.setup();
    const positions: Array<number | null> = [];
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeIndex={null}
        onActiveChange={(index) => positions.push(index)}
      />,
    );

    await user.tab();
    await user.keyboard("{ArrowRight}");

    expect(positions.at(-1)).toBeTypeOf("number");
  });

  it("says so plainly when a route has no elevation", () => {
    const flat: Position[] = [
      [8, 49],
      [8, 49.01],
    ];
    render(
      <ElevationProfile
        profile={buildProfile(flat)}
        title="No profile"
        activeIndex={null}
        onActiveChange={() => {}}
      />,
    );

    expect(screen.getByText(/no elevation data/i)).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("shows the elevation range in the readout before any interaction", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeIndex={null}
        onActiveChange={() => {}}
      />,
    );

    // 100 m to 295 m over the generated climb.
    expect(screen.getByText(/100–295 m/)).toBeInTheDocument();
  });
});

describe("steadyBands", () => {
  it("absorbs a short opening run into the run that follows it", () => {
    expect(steadyBands(samplesOf([2, 0, 0, 0, 0]))).toEqual([0, 0, 0, 0, 0]);
  });

  it("absorbs a short run into the run before it", () => {
    expect(steadyBands(samplesOf([0, 0, 0, 2, 0, 0, 0]))).toEqual([0, 0, 0, 0, 0, 0, 0]);
  });

  it("leaves a sustained opening run alone", () => {
    expect(steadyBands(samplesOf([2, 2, 2, 0, 0, 0]))).toEqual([2, 2, 2, 0, 0, 0]);
  });

  it("leaves a profile of one run alone, short or not", () => {
    expect(steadyBands(samplesOf([2, 2]))).toEqual([2, 2]);
  });
});
