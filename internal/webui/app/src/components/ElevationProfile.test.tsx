import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import type { ProfileSample } from "../lib/profile";
import { buildProfile } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES, summariseSurface } from "../lib/surface";
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
function Harness({
  title = "Eich Rundkurs 90",
  surface = null,
}: {
  title?: string;
  surface?: SurfaceSummary | null;
}) {
  const [activeIndex, setActiveIndex] = useState<number | null>(null);

  return (
    <ElevationProfile
      profile={buildProfile(climb())}
      title={title}
      surface={surface}
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

  it("names the ground under the position when the stage has been classified", async () => {
    const user = userEvent.setup();
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "gravel", startIndex: 0, endIndex: coordinates.length - 1 },
    ]);
    render(<Harness surface={surface} />);

    await user.tab();
    await user.keyboard("{ArrowRight}");

    expect(screen.getByText(/Gravel/)).toBeInTheDocument();
    expect(
      screen.getByRole("slider", { name: /Position along Eich/ }).getAttribute("aria-valuetext"),
    ).toMatch(/gravel$/);
  });

  // The strip is the reason the surface bar left the page header: it says which
  // part of the climb is made of what, which a summary bar cannot.
  it("draws the ground along the distance axis, in the order it is ridden", () => {
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "asphalt", startIndex: 0, endIndex: 19 },
      { kind: "gravel", startIndex: 20, endIndex: coordinates.length - 1 },
    ]);
    const { container } = render(<Harness surface={surface} />);

    const stretches = [...container.querySelectorAll(".elevation-profile__surface")];
    expect(stretches).toHaveLength(2);
    expect(stretches.map((line) => line.getAttribute("stroke"))).toEqual([
      SURFACE_STYLES.asphalt.colour,
      SURFACE_STYLES.gravel.colour,
    ]);

    // In route order, and meeting where one class hands over to the next.
    const [first, second] = stretches as [SVGLineElement, SVGLineElement];
    expect(Number(first.getAttribute("x1"))).toBe(0);
    expect(Number(first.getAttribute("x2"))).toBeCloseTo(Number(second.getAttribute("x1")), 5);
    expect(Number(second.getAttribute("x2"))).toBeGreaterThan(Number(second.getAttribute("x1")));
  });

  // The dash pattern is the channel that survives greyscale and colour
  // blindness, and it has to mean the same thing here as it does on the map.
  it("wears the same dash pattern a class wears on the map", () => {
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "gravel", startIndex: 0, endIndex: coordinates.length - 1 },
    ]);
    const { container } = render(<Harness surface={surface} />);

    const stretch = container.querySelector(".elevation-profile__surface");
    expect(stretch?.getAttribute("stroke-dasharray")).toBe(
      SURFACE_STYLES.gravel.dashes.map((dash) => dash * 7).join(" "),
    );
  });

  // The two lanes are one instrument: a pointer moving onto the strip must not
  // leave the scrub region and blank the readout it came to read.
  it("keeps the strip inside the region the pointer can scrub", () => {
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "gravel", startIndex: 0, endIndex: coordinates.length - 1 },
    ]);
    const withSurface = render(<Harness surface={surface} />);
    const scrubbed = withSurface.getByRole("slider").style.height;
    withSurface.unmount();

    const withoutSurface = render(<Harness />);
    const plotOnly = withoutSurface.getByRole("slider").style.height;

    expect(Number.parseFloat(scrubbed)).toBeGreaterThan(Number.parseFloat(plotOnly));
  });

  it("draws no strip on a stage nothing has classified", () => {
    const { container } = render(<Harness />);

    expect(container.querySelector(".elevation-profile__surface")).toBeNull();
  });

  it("leaves the readout as it was on a stage nothing has classified", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.tab();
    await user.keyboard("{ArrowRight}");

    expect(screen.queryByText(/Gravel|Asphalt|Unsurveyed/)).not.toBeInTheDocument();
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
