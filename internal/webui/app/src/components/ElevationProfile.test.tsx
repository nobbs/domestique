import { fireEvent, render, screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import userEvent from "@testing-library/user-event";
import { useMemo, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Position } from "../api/types";
import type { DistanceWindow, ProfileSample } from "../lib/profile";
import { buildProfile, buildWindowedProfile } from "../lib/profile";
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

/** About 4.3 km, and the number every pointer expectation below is a share of. */
const ROUTE_METRES = buildProfile(climb())?.totalDistanceMetres ?? 0;

/**
 * The plot's width in jsdom, where every element measures zero and the chart
 * falls back to MIN_WIDTH: 240 less the left and right padding.
 */
const PLOT_WIDTH = 182;

/**
 * Gives an element a box, because jsdom gives everything a box of nothing and
 * the chart quite correctly refuses to read a position off one.
 *
 * The width matches the plot's, so a pointer x-coordinate and an SVG
 * x-coordinate are the same number and geometry can be asserted directly.
 */
function measured(element: Element): Element {
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    right: PLOT_WIDTH,
    bottom: 100,
    width: PLOT_WIDTH,
    height: 100,
    toJSON: () => ({}),
  });

  return element;
}

/** Where along the route a pointer at this many pixels lands. */
function metresAt(pixels: number): number {
  return (pixels / PLOT_WIDTH) * ROUTE_METRES;
}

async function dragAcross(user: UserEvent, target: Element, from: number, to: number) {
  await user.pointer([
    { keys: "[MouseLeft>]", target, coords: { clientX: from, clientY: 20 } },
    { target, coords: { clientX: to, clientY: 20 } },
    { keys: "[/MouseLeft]", target, coords: { clientX: to, clientY: 20 } },
  ]);
}

/** The chart is controlled, so exercising it needs something to hold the value. */
function Harness({
  title = "Eich Rundkurs 90",
  surface = null,
}: {
  title?: string;
  surface?: SurfaceSummary | null;
}) {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);

  return (
    <ElevationProfile
      profile={buildProfile(climb())}
      title={title}
      surface={surface}
      activeMetres={activeMetres}
      onActiveChange={setActiveMetres}
    />
  );
}

/**
 * The chart wired the way the stage page wires it: a window chosen on the chart
 * comes back as a profile rebuilt over that stretch. Anything less would test
 * the drag against a chart that never redraws.
 */
function ZoomHarness({
  surface = null,
  onZoom,
}: {
  surface?: SurfaceSummary | null;
  onZoom?: (window: DistanceWindow | null) => void;
}) {
  const coordinates = useMemo(() => climb(), []);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const windowed = zoomWindow ? buildWindowedProfile(coordinates, zoomWindow) : null;

  return (
    <ElevationProfile
      profile={windowed ?? buildProfile(coordinates)}
      title="Eich Rundkurs 90"
      surface={surface}
      activeMetres={activeMetres}
      onActiveChange={setActiveMetres}
      zoomWindow={windowed ? zoomWindow : null}
      onZoomChange={(next) => {
        onZoom?.(next);
        setZoomWindow(next);
        setActiveMetres(null);
      }}
    />
  );
}

describe("ElevationProfile", () => {
  it("describes the profile for a reader who cannot see it", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeMetres={null}
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
        activeMetres={null}
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

  // The map means a place on the route by this, and a zoomed chart holds none of
  // the samples the whole-route one does, so the shared unit has to be ground.
  it("reports the scrubbed position as a distance, so the map can mark it", async () => {
    const user = userEvent.setup();
    const positions: Array<number | null> = [];
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeMetres={null}
        onActiveChange={(metres) => positions.push(metres)}
      />,
    );

    const scrub = measured(screen.getByRole("slider"));
    await user.pointer({ target: scrub, coords: { clientX: 91, clientY: 20 } });

    expect(positions.at(-1)).toBeCloseTo(metresAt(91), 0);
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
        activeMetres={null}
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
        activeMetres={null}
        onActiveChange={() => {}}
      />,
    );

    // 100 m to 295 m over the generated climb.
    expect(screen.getByText(/100–295 m/)).toBeInTheDocument();
  });
});

describe("ElevationProfile zooming", () => {
  it("zooms to the stretch the pointer was dragged across", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    expect(onZoom).toHaveBeenCalledTimes(1);
    const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
    expect(window.startMetres).toBeCloseTo(metresAt(20), 0);
    expect(window.endMetres).toBeCloseTo(metresAt(120), 0);
  });

  // A range is the ground between two points, not a direction of travel.
  it("reads a drag right to left the same as one left to right", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    await dragAcross(user, measured(screen.getByRole("slider")), 120, 20);

    const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
    expect(window.startMetres).toBeCloseTo(metresAt(20), 0);
    expect(window.endMetres).toBeCloseTo(metresAt(120), 0);
  });

  it("takes a hand that barely moved as scrubbing, not as a selection", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 23);

    expect(onZoom).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /Whole route/ })).not.toBeInTheDocument();
  });

  it("zooms by touch as well as by mouse", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    const scrub = measured(screen.getByRole("slider"));
    await user.pointer([
      { keys: "[TouchA>]", target: scrub, coords: { clientX: 30, clientY: 20 } },
      { pointerName: "TouchA", target: scrub, coords: { clientX: 140, clientY: 20 } },
      { keys: "[/TouchA]", target: scrub, coords: { clientX: 140, clientY: 20 } },
    ]);

    const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
    expect(window.startMetres).toBeCloseTo(metresAt(30), 0);
    expect(window.endMetres).toBeCloseTo(metresAt(140), 0);
  });

  it("shows the stretch under the pointer while it is still being chosen", async () => {
    const user = userEvent.setup();
    const { container } = render(<ZoomHarness />);
    const scrub = measured(screen.getByRole("slider"));

    await user.pointer([
      { keys: "[MouseLeft>]", target: scrub, coords: { clientX: 20, clientY: 20 } },
      { target: scrub, coords: { clientX: 120, clientY: 20 } },
    ]);

    const veils = [...container.querySelectorAll(".elevation-profile__veil")];
    expect(veils).toHaveLength(2);
    // Over what is being left behind on either side, and nothing in between.
    expect(Number(veils[0]?.getAttribute("width"))).toBeCloseTo(20, 0);
    expect(Number(veils[1]?.getAttribute("x"))).toBeCloseTo(120, 0);

    await user.pointer({ keys: "[/MouseLeft]", target: scrub, coords: { clientX: 120 } });
    expect(container.querySelector(".elevation-profile__veil")).toBeNull();
  });

  it("commits nothing when the gesture is cancelled out from under it", () => {
    const onZoom = vi.fn();
    const { container } = render(<ZoomHarness onZoom={onZoom} />);
    const scrub = measured(screen.getByRole("slider"));

    fireEvent.pointerDown(scrub, { pointerId: 1, isPrimary: true, button: 0, clientX: 20 });
    fireEvent.pointerMove(scrub, { pointerId: 1, clientX: 120 });
    fireEvent.pointerCancel(scrub, { pointerId: 1 });

    expect(onZoom).not.toHaveBeenCalled();
    expect(container.querySelector(".elevation-profile__veil")).toBeNull();
  });

  // The reader asked to look closer at somewhere. The answer to "closer than the
  // data goes" is the closest the data goes, not nothing at all.
  it("grows a selection too short to plot rather than refusing it", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 28);

    const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
    expect(metresAt(28) - metresAt(20)).toBeLessThan(200);
    expect(window.endMetres - window.startMetres).toBeCloseTo(200, 5);
    // Grown about the middle of what was actually drawn.
    expect((window.startMetres + window.endMetres) / 2).toBeCloseTo(metresAt(24), 0);
  });

  it("offers the way back only once there is somewhere to go back to", async () => {
    const user = userEvent.setup();
    render(<ZoomHarness />);

    expect(screen.queryByRole("button", { name: /Whole route/ })).not.toBeInTheDocument();

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    expect(screen.getByRole("button", { name: /Whole route/ })).toHaveAttribute(
      "aria-keyshortcuts",
      "Escape",
    );
  });

  it("says which stretch of the route it is showing", async () => {
    const user = userEvent.setup();
    render(<ZoomHarness />);

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    const shown = /0\.5–2\.9 km/;
    expect(screen.getByRole("button", { name: /Whole route/ })).toHaveAccessibleName(shown);
    expect(screen.getByRole("slider")).toHaveAccessibleName(shown);
    expect(screen.getByRole("img")).toHaveAccessibleName(shown);
    expect(Number(screen.getByRole("slider").getAttribute("aria-valuemin"))).toBeCloseTo(0.5, 5);
  });

  it("returns to the whole route when the way back is taken", async () => {
    const user = userEvent.setup();
    render(<ZoomHarness />);
    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    await user.click(screen.getByRole("button", { name: /Whole route/ }));

    expect(screen.queryByRole("button", { name: /Whole route/ })).not.toBeInTheDocument();
    expect(screen.getByRole("slider")).toHaveAttribute("aria-valuemin", "0");
  });

  // The gesture that zooms is a drag, which leaves focus wherever it began, so
  // the way out has to work from wherever the reader is.
  it("returns to the whole route on Escape, from anywhere on the page", async () => {
    const user = userEvent.setup();
    render(<ZoomHarness />);
    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("button", { name: /Whole route/ })).not.toBeInTheDocument();
  });

  it("leaves Escape alone while the whole route is on show", async () => {
    const user = userEvent.setup();
    const onZoom = vi.fn();
    render(<ZoomHarness onZoom={onZoom} />);

    await user.keyboard("{Escape}");

    expect(onZoom).not.toHaveBeenCalled();
  });

  it("marks nothing when the map reports a position outside the stretch on show", () => {
    const coordinates = climb();
    const window = { startMetres: 1000, endMetres: 3000 };
    const { container } = render(
      <ElevationProfile
        profile={buildWindowedProfile(coordinates, window)}
        title="Eich Rundkurs 90"
        activeMetres={200}
        onActiveChange={() => {}}
        zoomWindow={window}
        onZoomChange={() => {}}
      />,
    );

    expect(container.querySelector(".elevation-profile__cursor")).toBeNull();
  });

  it("steps within the stretch on show, not across the whole route", async () => {
    const user = userEvent.setup();
    const positions: number[] = [];
    const window = { startMetres: 1000, endMetres: 3000 };
    render(
      <ElevationProfile
        profile={buildWindowedProfile(climb(), window)}
        title="Eich Rundkurs 90"
        activeMetres={null}
        onActiveChange={(metres) => metres !== null && positions.push(metres)}
        zoomWindow={window}
        onZoomChange={() => {}}
      />,
    );

    await user.tab();
    await user.keyboard("{ArrowLeft}");
    await user.keyboard("{ArrowRight}");

    expect(Math.min(...positions)).toBeGreaterThanOrEqual(1000);
    expect(Math.max(...positions)).toBeLessThanOrEqual(3000);
  });

  it("clips the surface strip to the stretch on show", () => {
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "asphalt", startIndex: 0, endIndex: 19 },
      { kind: "gravel", startIndex: 20, endIndex: coordinates.length - 1 },
    ]);
    const window = { startMetres: 3000, endMetres: 4000 };
    const { container } = render(
      <ElevationProfile
        profile={buildWindowedProfile(coordinates, window)}
        title="Eich Rundkurs 90"
        surface={surface}
        activeMetres={null}
        onActiveChange={() => {}}
        zoomWindow={window}
        onZoomChange={() => {}}
      />,
    );

    const stretches = [...container.querySelectorAll(".elevation-profile__surface")];
    expect(stretches.map((line) => line.getAttribute("stroke"))).toEqual([
      SURFACE_STYLES.gravel.colour,
    ]);
    // Cut at the window's edge, not drawn from where the class actually starts.
    expect(Number(stretches[0]?.getAttribute("x1"))).toBe(0);
  });

  // Zooming answers a question about the terrain; it must not also resize the
  // instrument under the reader's hand.
  it("keeps the instrument the same size zoomed in as zoomed out", async () => {
    const user = userEvent.setup();
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "gravel", startIndex: 0, endIndex: coordinates.length - 1 },
    ]);
    render(<ZoomHarness surface={surface} />);
    const whole = screen.getByRole("slider").style.height;

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 120);

    expect(screen.getByRole("slider").style.height).toBe(whole);
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
