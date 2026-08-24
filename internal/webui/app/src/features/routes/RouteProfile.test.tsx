import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import { buildProfile } from "../../lib/profile";
import { RouteProfile } from "./RouteProfile";

/** A climb from 100 m to 295 m, so both ends of the range are worth reading. */
function climb(): Position[] {
  return Array.from(
    { length: 40 },
    (_, index): Position => [8, 49 + index * 0.001, 100 + index * 5],
  );
}

const PROFILE = buildProfile(climb());

/**
 * Answers the coarse-pointer query yes, which the shared setup answers no.
 *
 * The hint over the plot is the one thing that reads the pointer, and the two
 * answers are two different sentences, so both have to be renderable here.
 */
function stubCoarsePointer() {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: query.includes("pointer: coarse"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

function show(props: Partial<React.ComponentProps<typeof RouteProfile>> = {}) {
  const onCollapsedChange = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // The strip draws nothing until its forecast has arrived, so a test about
  // what the strip is handed has to put one in the cache first.
  const samples = props.samples ?? [];
  if (samples.length > 0) {
    client.setQueryData(weatherQuery(samples).queryKey, {
      points: samples.map(() => ({
        time: new Date("2026-08-25T07:00:00Z").toISOString(),
        temperatureCelsius: 14,
        apparentTemperatureCelsius: 12,
        precipitationMillimetres: 0,
        precipitationProbabilityPercent: 0,
        windSpeedKmh: 10,
        windDirectionDegrees: 180,
        weatherCode: 1,
      })),
    });
  }
  render(
    <QueryClientProvider client={client}>
      <RouteProfile
        profile={PROFILE}
        title="Col du Test"
        ascentMetres={1560}
        surface={null}
        activeMetres={null}
        onActiveChange={() => {}}
        zoomWindow={null}
        onZoomChange={() => {}}
        highlight={null}
        collapsed={false}
        onCollapsedChange={onCollapsedChange}
        unitSystem="metric"
        startAt={null}
        onStartAtChange={() => {}}
        samples={[]}
        coordinates={climb()}
        {...props}
      />
    </QueryClientProvider>,
  );

  return { onCollapsedChange };
}

/** The line beside the heading, whatever it happens to be saying. */
function summary(): string {
  return document.querySelector(".route-profile__summary")?.textContent ?? "";
}

beforeEach(() => {
  // The forecast strip owns its own query; a start time this suite is not
  // exercising directly must still find a fetch stub loud enough to notice an
  // unseeded request rather than hang.
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("RouteProfile", () => {
  it("is a section of the card rather than a panel of its own", () => {
    show();

    expect(screen.getByRole("region", { name: "Elevation" })).toContainElement(
      document.querySelector("#elevation-plot"),
    );
  });

  it("says what a mouse can do with the chart", () => {
    show();

    expect(summary()).toBe("100 m–295 m · drag across to look closer");
  });

  // A finger cannot press and drag in one movement without taking the scroll
  // with it, so the gesture is a different one and has to be named differently.
  it("says what a finger can do with the chart instead", () => {
    stubCoarsePointer();

    show();

    expect(summary()).toBe("100 m–295 m · press and hold to look closer");
  });

  it("says which stretch is on show, and the way back out of it", () => {
    show({ zoomWindow: { startMetres: 500, endMetres: 2900 } });

    expect(summary()).toBe("0.5–2.9 km shown · Escape returns");
  });

  it("reports the same figures in feet and miles for the imperial system", () => {
    show({ unitSystem: "imperial", zoomWindow: { startMetres: 500, endMetres: 2900 } });

    expect(summary()).toBe("0.3–1.8 mi shown · Escape returns");
  });

  // Nothing to plot is not the same as a plot with nothing in it: a route the
  // service has no elevation for should not be offered a hint about scrubbing.
  it("offers no hint at all for a route with no profile", () => {
    show({ profile: null });

    expect(document.querySelector(".route-profile__summary")).toBeNull();
  });

  it("folds to a row still carrying the figures the chart was there to give", () => {
    show({ collapsed: true });

    expect(summary()).toBe("100 m–295 m · 1,560 m up");
    expect(document.querySelector("#elevation-plot")).toBeNull();
  });

  // A flat ride climbs nothing, and "0 m up" is a figure that reads as a
  // measurement failure rather than as a flat ride.
  it("leaves the climbing out of that row when there is none to give", () => {
    show({ collapsed: true, ascentMetres: 0 });

    expect(summary()).toBe("100 m–295 m");
  });

  it("says nothing in that row for a route it has no figures for", () => {
    show({ collapsed: true, profile: null, ascentMetres: 0 });

    expect(document.querySelector(".route-profile__summary")).toBeNull();
  });

  it("carries the words the chevron does not, and points at the plot it folds", async () => {
    const { onCollapsedChange } = show();
    const fold = screen.getByRole("button", { name: "Hide the profile" });

    expect(fold).toHaveAttribute("aria-expanded", "true");
    expect(fold).toHaveAttribute("aria-controls", "elevation-plot");

    await userEvent.click(fold);

    expect(onCollapsedChange).toHaveBeenCalledWith(true);
  });

  // The plot is unmounted when the row is folded, and a control pointing at an
  // element that is not in the document is a reference nobody can follow.
  it("points at nothing once there is no plot to point at", () => {
    show({ collapsed: true });
    const unfold = screen.getByRole("button", { name: "Show the profile" });

    expect(unfold).toHaveAttribute("aria-expanded", "false");
    expect(unfold).not.toHaveAttribute("aria-controls");
  });

  it("offers a way to set a ride start time, beside the chart", () => {
    show();

    expect(screen.getByLabelText("Ride start")).toBeInTheDocument();
  });

  // Answering "when the strip is asking about" for a chart that is not on the
  // page any more is a question nobody there asked.
  it("folds the start-time control away with the chart", () => {
    show({ collapsed: true });

    expect(screen.queryByLabelText("Ride start")).not.toBeInTheDocument();
  });

  // No default start time means no forecast strip until the reader chooses
  // one — see lib/startTime.ts on why a chosen time is never invented.
  it("draws no forecast strip before a start time is chosen", () => {
    show({ startAt: null, samples: [] });

    expect(document.querySelector(".forecast-strip")).toBeNull();
  });

  /*
   * A stage with a profile but no predicted moving time has no timeline for a
   * forecast to hang on. The reader has asked for one by picking a start, so
   * the absence is owed a reason rather than an empty space where a strip
   * would be — and never a guessed speed to fill the gap.
   */
  it("explains an absent forecast on a stage nothing has predicted", () => {
    show({ startAt: new Date("2026-08-25T07:00:00Z"), samples: [], rideSeconds: undefined });

    expect(screen.getByText(/no predicted moving time/i)).toBeInTheDocument();
    expect(document.querySelector(".forecast-strip")).toBeNull();
  });

  /*
   * A start time is remembered across visits, so the common path is opening a
   * stage with one already set. `rideSeconds` is undefined while the geometry
   * is in flight and also when the answer carries no prediction, and announcing
   * the second during the first would tell a reader their stage has no
   * prediction for as long as the fetch takes.
   */
  it("waits for the geometry before declaring a stage unpredicted", () => {
    show({
      startAt: new Date(Date.now() + 60 * 60 * 1000),
      samples: [],
      rideSeconds: undefined,
      predictionKnown: false,
    });

    expect(screen.queryByText(/no predicted moving time/i)).not.toBeInTheDocument();
  });

  it("says nothing of the sort once the stage has a prediction", () => {
    show({ startAt: new Date("2026-08-25T07:00:00Z"), samples: [], rideSeconds: 3600 });

    expect(screen.queryByText(/no predicted moving time/i)).not.toBeInTheDocument();
  });

  /*
   * The picker's bounds constrain what a reader can type, and say nothing
   * about a value handed back from storage. A start remembered from an earlier
   * visit can age past the 24-hour allowance while a page sits open, and one
   * that fits a short stage can put a long stage's finish past the horizon.
   * Sending it earns a 400 the strip can only report as the provider being
   * down, so it is caught here and explained instead.
   */
  it("explains a remembered start the forecast window has moved past", () => {
    const longAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000);

    show({ startAt: longAgo, samples: [], rideSeconds: 3600 });

    // The remedy for a stale start is a later one, so it must not be told its
    // ride finishes past the horizon — that would send the reader backwards.
    expect(screen.getByText(/more than a day in the past/i)).toBeInTheDocument();
    expect(screen.queryByText(/16-day forecast window/i)).not.toBeInTheDocument();
    expect(document.querySelector(".forecast-strip")).toBeNull();
  });

  it("explains a start whose finish falls past the horizon on this stage", () => {
    // Inside the window at the start line, and past it ten hours later.
    const nearlyTheHorizon = new Date(Date.now() + 16 * 24 * 60 * 60 * 1000 - 60 * 60 * 1000);

    show({ startAt: nearlyTheHorizon, samples: [], rideSeconds: 10 * 60 * 60 });

    expect(screen.getByText(/outside the 16-day forecast window/i)).toBeInTheDocument();
  });

  /*
   * Weather needs no terrain; only the shared axis does. A stage with a
   * timeline but no profile therefore falls back to the whole route rather
   * than a window of zero length, which nothing would overlap and every cell
   * would be dropped against. Today no such stage exists — a prediction needs
   * complete elevation — so this pins the fallback before something else
   * starts producing timelines.
   */
  it("spans the whole route when there is no profile to share an axis with", () => {
    const coordinates = climb();
    const samples = [
      { position: coordinates[0] as Position, distanceMetres: 0, arrivalAt: new Date() },
      {
        position: coordinates[coordinates.length - 1] as Position,
        distanceMetres: 4_000,
        arrivalAt: new Date(),
      },
    ];

    show({
      profile: null,
      startAt: new Date("2026-08-25T07:00:00Z"),
      samples,
      rideSeconds: 3600,
      coordinates,
    });

    const strip = document.querySelector(".forecast-strip svg");
    expect(strip?.getAttribute("viewBox")).not.toMatch(/^0 0 0/);
  });
});
