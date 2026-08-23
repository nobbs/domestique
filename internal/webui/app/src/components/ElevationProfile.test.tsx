import { act, fireEvent, render, screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import userEvent from "@testing-library/user-event";
import { useMemo, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Position } from "../api/types";
import type { Highlight } from "../lib/highlight";
import type { DistanceWindow } from "../lib/profile";
import { buildProfile, buildWindowedProfile } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { summariseSurface } from "../lib/surface";
import { ElevationProfile, LONG_PRESS_MS } from "./ElevationProfile";

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
const PLOT_WIDTH = 192;

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
  highlight = null,
}: {
  title?: string;
  surface?: SurfaceSummary | null;
  highlight?: Highlight | null;
}) {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);

  return (
    <ElevationProfile
      profile={buildProfile(climb())}
      title={title}
      surface={surface}
      activeMetres={activeMetres}
      onActiveChange={setActiveMetres}
      highlight={highlight}
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
  unitSystem = "metric",
}: {
  surface?: SurfaceSummary | null;
  onZoom?: (window: DistanceWindow | null) => void;
  unitSystem?: "metric" | "imperial";
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
      unitSystem={unitSystem}
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

  /*
   * The strip the chart used to draw along its foot is gone with the panel it
   * had room on. Inside the card the surface mix is a bar with chips two rows
   * below the plot, and a second chart on the same axis saying the same thing
   * would be the row that made the card too tall to read.
   */
  it("draws no ground along the chart, whether or not the route is classified", () => {
    const coordinates = climb();
    const surface = summariseSurface(coordinates, [
      { kind: "asphalt", startIndex: 0, endIndex: 19 },
      { kind: "gravel", startIndex: 20, endIndex: coordinates.length - 1 },
    ]);

    const classified = render(<Harness surface={surface} />);
    expect(classified.container.querySelector(".elevation-profile__surface")).toBeNull();
    classified.unmount();

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

  it("reports the same range in feet for the imperial system", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeMetres={null}
        onActiveChange={() => {}}
        unitSystem="imperial"
      />,
    );

    // 100 m to 295 m converts to 328 ft and 968 ft.
    expect(screen.getByText(/328–968 ft/)).toBeInTheDocument();
  });

  it("says the summary in miles and feet for the imperial system", () => {
    render(
      <ElevationProfile
        profile={buildProfile(climb())}
        title="Eich Rundkurs 90"
        activeMetres={null}
        onActiveChange={() => {}}
        unitSystem="imperial"
      />,
    );

    const figure = screen.getByRole("img", { name: /Eich Rundkurs 90/ });
    expect(figure).toHaveAccessibleName(/miles/);
    expect(figure).toHaveAccessibleName(/feet above sea level/);
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

  /*
   * A finger gets the same two gestures, but has to ask for them first: the
   * chart is a row of a card that scrolls, so a touch belongs to the card until
   * it has been held long enough to be a question about the climb.
   */
  it("zooms by touch, once the hold has armed the gesture", () => {
    vi.useFakeTimers();
    try {
      const onZoom = vi.fn();
      const { container } = render(<ZoomHarness onZoom={onZoom} />);
      const scrub = measured(screen.getByRole("slider"));

      fireEvent.pointerDown(scrub, {
        pointerId: 3,
        pointerType: "touch",
        isPrimary: true,
        button: 0,
        clientX: 30,
      });
      // Still the card's, and the chart says so: a swipe from here scrolls.
      expect(scrub.getAttribute("data-holding")).toBeNull();

      act(() => {
        vi.advanceTimersByTime(LONG_PRESS_MS);
      });
      expect(scrub.getAttribute("data-holding")).toBe("true");

      fireEvent.pointerMove(scrub, { pointerId: 3, pointerType: "touch", clientX: 140 });
      fireEvent.pointerUp(scrub, { pointerId: 3, pointerType: "touch", clientX: 140 });

      const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
      expect(window.startMetres).toBeCloseTo(metresAt(30), 0);
      expect(window.endMetres).toBeCloseTo(metresAt(140), 0);
      expect(container.querySelector(".elevation-profile__veil")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  // A finger that moved before the hold completed was scrolling past, and a
  // gesture armed under it would take the card's scroll away mid-swipe.
  it("gives a touch that moved on back to the card", () => {
    vi.useFakeTimers();
    try {
      const onZoom = vi.fn();
      render(<ZoomHarness onZoom={onZoom} />);
      const scrub = measured(screen.getByRole("slider"));

      fireEvent.pointerDown(scrub, {
        pointerId: 4,
        pointerType: "touch",
        isPrimary: true,
        button: 0,
        clientX: 30,
      });
      fireEvent.pointerMove(scrub, { pointerId: 4, pointerType: "touch", clientX: 60 });
      act(() => {
        vi.advanceTimersByTime(LONG_PRESS_MS * 2);
      });
      fireEvent.pointerMove(scrub, { pointerId: 4, pointerType: "touch", clientX: 140 });
      fireEvent.pointerUp(scrub, { pointerId: 4, pointerType: "touch", clientX: 140 });

      expect(scrub.getAttribute("data-holding")).toBeNull();
      expect(onZoom).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  /*
   * A press does not always arrive after the release of the one before it. A
   * device with both a touchscreen and a trackpad can put a second primary
   * pointer down mid-hold, and a timer left over from the first would fire into
   * the second: the chart would capture a pointer and anchor a drag at a
   * position the reader had already left.
   */
  it("drops a hold still counting when a fresh press lands on the plot", () => {
    vi.useFakeTimers();
    try {
      const onZoom = vi.fn();
      render(<ZoomHarness onZoom={onZoom} />);
      const scrub = measured(screen.getByRole("slider"));

      fireEvent.pointerDown(scrub, {
        pointerId: 5,
        pointerType: "touch",
        isPrimary: true,
        button: 0,
        clientX: 30,
      });
      fireEvent.pointerDown(scrub, {
        pointerId: 6,
        pointerType: "mouse",
        isPrimary: true,
        button: 0,
        clientX: 140,
      });
      act(() => {
        vi.advanceTimersByTime(LONG_PRESS_MS * 2);
      });

      // The abandoned touch never arms, so the mouse's own drag is the only one
      // in play and it is anchored where the mouse actually went down.
      expect(scrub.getAttribute("data-holding")).toBeNull();

      fireEvent.pointerMove(scrub, { pointerId: 6, pointerType: "mouse", clientX: 60 });
      fireEvent.pointerUp(scrub, { pointerId: 6, pointerType: "mouse", clientX: 60 });

      const window = onZoom.mock.calls[0]?.[0] as DistanceWindow;
      expect(window.startMetres).toBeCloseTo(metresAt(60), 0);
      expect(window.endMetres).toBeCloseTo(metresAt(140), 0);
    } finally {
      vi.useRealTimers();
    }
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

    const shown = /0\.5–2\.7 km/;
    expect(screen.getByRole("button", { name: /Whole route/ })).toHaveAccessibleName(shown);
    expect(screen.getByRole("slider")).toHaveAccessibleName(shown);
    expect(screen.getByRole("img")).toHaveAccessibleName(shown);
    expect(Number(screen.getByRole("slider").getAttribute("aria-valuemin"))).toBeCloseTo(0.5, 5);
  });

  /*
   * A mile is coarse enough that a fixed tenth can hold still for a couple of
   * hundred metres of drag — the same adaptive precision the axis labels use
   * has to carry `aria-valuenow` too, or a fine zoom in imperial announces the
   * same position for every step a reader takes.
   */
  it("keeps enough precision in imperial for a fine zoom's positions to read apart", async () => {
    const user = userEvent.setup();
    render(<ZoomHarness unitSystem="imperial" />);

    await dragAcross(user, measured(screen.getByRole("slider")), 20, 40);

    const scrub = screen.getByRole("slider");
    scrub.focus();
    fireEvent.keyDown(scrub, { key: "ArrowRight" });
    const first = scrub.getAttribute("aria-valuenow");
    fireEvent.keyDown(scrub, { key: "ArrowRight" });
    const second = scrub.getAttribute("aria-valuenow");

    expect(first).not.toBe(second);
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

  // A slider that declares a range and then reports a value outside it is not
  // something assistive technology can place on the route.
  it("rests at the start of the stretch on show, not at the start of the route", () => {
    const window = { startMetres: 1000, endMetres: 3000 };
    render(
      <ElevationProfile
        profile={buildWindowedProfile(climb(), window)}
        title="Eich Rundkurs 90"
        activeMetres={null}
        onActiveChange={() => {}}
        zoomWindow={window}
        onZoomChange={() => {}}
      />,
    );

    const scrub = screen.getByRole("slider");
    const now = Number(scrub.getAttribute("aria-valuenow"));
    expect(now).toBeGreaterThanOrEqual(Number(scrub.getAttribute("aria-valuemin")));
    expect(now).toBeLessThanOrEqual(Number(scrub.getAttribute("aria-valuemax")));
    expect(scrub).toHaveAttribute("aria-valuetext", "No position selected");
  });

  // Without capture the pointerup lands somewhere else entirely, so a band left
  // painted here would outlive the gesture that drew it.
  it("ends a drag the chart can no longer follow when the pointer leaves it", () => {
    const onZoom = vi.fn();
    const { container } = render(<ZoomHarness onZoom={onZoom} />);
    const scrub = measured(screen.getByRole("slider"));

    fireEvent.pointerDown(scrub, { pointerId: 1, isPrimary: true, button: 0, clientX: 20 });
    fireEvent.pointerMove(scrub, { pointerId: 1, clientX: 120 });
    expect(container.querySelector(".elevation-profile__veil")).not.toBeNull();

    fireEvent.pointerLeave(scrub, { pointerId: 1 });

    expect(container.querySelector(".elevation-profile__veil")).toBeNull();
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

/*
 * The test climb runs at a steady four and a half percent, so it is band 1 from
 * end to end — which makes it both a stage that is entirely the picked class and
 * a stage with none of any other.
 */
describe("a picked class", () => {
  it("leaves the chart unveiled when nothing is picked", () => {
    const { container } = render(<Harness />);

    expect(container.querySelectorAll(".elevation-profile__veil")).toHaveLength(0);
  });

  it("veils nothing when the whole stage is the picked band", () => {
    const { container } = render(<Harness highlight={{ type: "band", band: 1 }} />);

    expect(container.querySelectorAll(".elevation-profile__veil")).toHaveLength(0);
  });

  it("veils the whole chart when the stage has none of the picked band", () => {
    const { container } = render(<Harness highlight={{ type: "band", band: 4 }} />);

    expect(container.querySelectorAll(".elevation-profile__veil")).toHaveLength(1);
  });

  it("lights only the picked class, and leaves the rest of the ride veiled", () => {
    const summary = summariseSurface(climb(), [
      { kind: "asphalt", startIndex: 0, endIndex: 18 },
      { kind: "gravel", startIndex: 19, endIndex: 38 },
    ]);
    const { container } = render(
      <Harness surface={summary} highlight={{ type: "surface", kind: "gravel" }} />,
    );

    expect(container.querySelectorAll(".elevation-profile__veil")).toHaveLength(1);
  });

  // A chart that is mostly veiled has to explain itself to a reader who cannot
  // see the veil at all.
  it("names the picked class in the spoken summary", () => {
    render(<Harness highlight={{ type: "band", band: 1 }} />);

    expect(screen.getByRole("img")).toHaveAccessibleName(/Only the 3 to 6% stretches are lit\./);
  });
});
