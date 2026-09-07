/**
 * One ride's page, without a canvas.
 *
 * The map needs WebGL and the chart needs a laid-out DOM, so both are stood in
 * for by fakes that record what they were handed. What is asserted is the
 * agreement the page keeps: the ride it names is the ride it draws, and the
 * altitudes that arrive with the track are what the profile is built from.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  activitiesQuery,
  activitySeriesQuery,
  activitySplitsQuery,
  activityTrackQuery,
  webUIConfigQuery,
} from "../../api/queries";
import type {
  Activity,
  ActivitySplit,
  ActivityTrack,
  Position,
  WebUIConfig,
} from "../../api/types";
import type { Profile } from "../../lib/profile";
import type { AlignedSeries } from "../../lib/rideSeries";
import { ActivityPage } from "./ActivityPage";

const ZONE = "Europe/Berlin";

const drawn = vi.hoisted(() => ({
  coordinates: [] as Position[],
  bounds: null as number[] | null,
  profiles: [] as Array<Profile | null>,
  series: [] as AlignedSeries[],
}));

vi.mock("./ActivityMap", () => ({
  ActivityMap: (props: { coordinates: Position[]; bounds: number[]; profile: Profile | null }) => {
    drawn.coordinates = props.coordinates;
    drawn.bounds = props.bounds;

    return <div data-testid="activity-map" />;
  },
}));

vi.mock("../routes/ElevationProfile", () => ({
  ElevationProfile: (props: { profile: Profile | null; series?: AlignedSeries[] }) => {
    drawn.profiles.push(props.profile);
    drawn.series = props.series ?? [];

    return <div data-testid="elevation-profile" />;
  },
}));

const RIDE: Activity = {
  id: 7,
  startedAt: "2026-08-26T08:00:00Z",
  distanceMetres: 30_000,
  movingSeconds: 3_600,
  elapsedSeconds: 4_000,
  ascentMetres: 300,
  typeId: 40,
  locationId: 0,
};

/** A track as the query hands it over, with or without altitudes. */
function track(withAltitude = true): ActivityTrack {
  const positions: Position[] = withAltitude
    ? [
        [8.4, 49, 110],
        [8.5, 49.1, 180],
        [8.6, 49.2, 140],
      ]
    : [
        [8.4, 49],
        [8.5, 49.1],
        [8.6, 49.2],
      ];

  return { bbox: [8.4, 49, 8.6, 49.2], coordinates: positions, state: "stored" };
}

/** A track whose barometer had not settled for the first two samples. */
function trackWithLeadingGap(): ActivityTrack {
  return {
    bbox: [8.4, 49, 8.6, 49.2],
    coordinates: [
      [8.3, 48.9],
      [8.4, 49],
      [8.5, 49.1, 180],
      [8.6, 49.2, 140],
    ],
    state: "stored",
  };
}

function config(): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: ZONE,
    identity: { display: "rider@example.test", admin: false },
  };
}

function show(
  recorded: ActivityTrack | null = track(),
  activityId: number | string = RIDE.id,
  heartRate?: (number | null)[],
  ride: Activity = RIDE,
  splits: ActivitySplit[] = [],
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config());
  client.setQueryData(activitiesQuery().queryKey, [ride]);
  // The page's own guard: only a run of digits names a ride. Seeding under the
  // id the route actually carries is what keeps a test off the network, since
  // any key the page does not ask for leaves its query to fetch for real.
  const asked = /^\d+$/.test(String(activityId)) ? Number(activityId) : null;
  if (asked !== null) {
    client.setQueryData(activitySplitsQuery(asked).queryKey, { splits });
    if (recorded) {
      client.setQueryData(activityTrackQuery(asked).queryKey, recorded);
    }
    if (heartRate) {
      client.setQueryData(activitySeriesQuery(asked, "heartRate").queryKey, {
        series: "heartRate",
        values: heartRate,
      });
    }
  }
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/activities/${activityId}`]}>
        <Routes>
          <Route path="activities/:activityId" element={<ActivityPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/**
 * Every activity request this page makes is seeded, so one going out means a
 * query was missed and a test would be reaching for the network. React Query
 * swallows a throwing fetch, so what catches it is counting the calls.
 */
const fetched = vi.fn((target: unknown) =>
  Promise.reject(new Error(`a test asked for the network: ${String(target)}`)),
);

/** The activity paths asked for, which should always be none. */
function activityRequests(): string[] {
  return fetched.mock.calls
    .map(([target]) => String(target))
    .filter((path) => path.startsWith("/v1/activities"));
}

beforeEach(() => {
  drawn.coordinates = [];
  drawn.bounds = null;
  drawn.profiles = [];
  drawn.series = [];
  fetched.mockClear();
  vi.stubGlobal("fetch", fetched);
});

afterEach(() => {
  expect(activityRequests()).toEqual([]);
  vi.unstubAllGlobals();
});

describe("one ride's page", () => {
  // Measured, not predicted: the header used to round a ride's own moving time
  // to five minutes, which is a route estimate's manner rather than a record's.
  it("shows the ride's moving and elapsed times as measured", () => {
    show();

    expect(screen.getByText(/1 h moving/)).toBeInTheDocument();
    expect(screen.getByText(/1 h 6 min elapsed/)).toBeInTheDocument();
  });

  it("hands the ride's fetched splits to the table", () => {
    show(track(), RIDE.id, undefined, RIDE, [
      { distanceMetres: 1000, movingSeconds: 120, ascentMetres: 0 },
      { distanceMetres: 500, movingSeconds: 90, ascentMetres: 0 },
    ]);

    expect(screen.getByLabelText("Splits")).toBeInTheDocument();
    expect(screen.getByText("1.5 km")).toBeInTheDocument();
  });

  // A ride the service cut into no stretches shows no table at all.
  it("shows no splits table for a ride with none", () => {
    show();

    expect(screen.queryByLabelText("Splits")).not.toBeInTheDocument();
  });

  // Two near-identical figures say less than one.
  it("leaves out elapsed time for a ride that barely stopped", () => {
    show(track(), RIDE.id, undefined, { ...RIDE, elapsedSeconds: RIDE.movingSeconds + 30 });

    expect(screen.getByText(/1 h moving/)).toBeInTheDocument();
    expect(screen.queryByText(/elapsed/)).not.toBeInTheDocument();
  });

  it("names the ride and hands its track to the map", () => {
    show();

    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("2026");
    expect(screen.getByText(/30\.0 km/)).toBeInTheDocument();
    expect(screen.getByTestId("activity-map")).toBeInTheDocument();
    expect(drawn.bounds).toEqual([8.4, 49, 8.6, 49.2]);
    expect(drawn.coordinates).toEqual([
      [8.4, 49, 110],
      [8.5, 49.1, 180],
      [8.6, 49.2, 140],
    ]);
  });

  it("draws a profile from the altitudes the track carried", () => {
    show();

    expect(screen.getByTestId("elevation-profile")).toBeInTheDocument();
    expect(drawn.profiles.at(-1)?.samples.length).toBeGreaterThan(0);
  });

  // A ride recorded without altitudes is still a ride worth seeing on a map.
  it("draws the track alone when no altitude was recorded", () => {
    show(track(false));

    expect(screen.getByTestId("activity-map")).toBeInTheDocument();
    expect(screen.queryByTestId("elevation-profile")).not.toBeInTheDocument();
  });

  // The barometer warm-up leaves a leading run with no altitude on most rides;
  // that must not cost the whole ride its profile.
  it("draws a profile even when the recording starts before altitude was available", () => {
    show(trackWithLeadingGap());

    expect(screen.getByTestId("elevation-profile")).toBeInTheDocument();
    expect(drawn.profiles.at(-1)?.samples.length).toBeGreaterThan(0);
  });

  // Nothing is fetched until the rider asks: a ride can hold twenty thousand
  // samples, and five series nobody looked at would cost more than the track.
  it("draws no series until one is asked for", () => {
    show(track(), RIDE.id, [120, 148, 130]);

    expect(drawn.series).toEqual([]);
  });

  it("draws a series the rider turned on, over the profile's own samples", async () => {
    show(track(), RIDE.id, [120, 148, 130]);

    await userEvent.click(screen.getByRole("button", { name: /Heart rate/ }));

    expect(drawn.series).toHaveLength(1);
    expect(drawn.series[0]?.key).toBe("heartRate");
    expect(drawn.series[0]?.values).toHaveLength(drawn.profiles.at(-1)?.samples.length ?? 0);
    expect(drawn.series[0]?.values[0]).toBe(120);
  });

  it("puts the series away again when its chip is pressed a second time", async () => {
    show(track(), RIDE.id, [120, 148, 130]);

    const chip = screen.getByRole("button", { name: /Heart rate/ });
    await userEvent.click(chip);
    await userEvent.click(chip);

    expect(drawn.series).toEqual([]);
  });

  // The endpoint answers 404 for a series the ride never carried, and that is
  // the only failure that may read as "not recorded".
  it("says a series the ride never recorded is not there", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ error: { code: "not_found", message: "no such series" } }, { status: 404 }),
      ),
    );
    show();

    await userEvent.click(screen.getByRole("button", { name: /Power/ }));

    expect(await screen.findByRole("button", { name: /Power.*not recorded/ })).toBeInTheDocument();
  });

  // Anything else is about the service, not about the bicycle.
  it("does not read an unreachable service as a series the ride never recorded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ error: { code: "unavailable", message: "try later" } }, { status: 503 }),
      ),
    );
    show();

    await userEvent.click(screen.getByRole("button", { name: /Heart rate/ }));

    expect(
      await screen.findByRole("button", { name: /Heart rate.*unavailable/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Heart rate/ }).textContent).not.toContain(
      "not recorded",
    );
  });

  it("shows a placeholder while the track is still loading", () => {
    // Uncached, so React Query falls through to a real fetch; stub it so the
    // request never settles and the page stays in its loading state.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => {})),
    );
    show(null);

    expect(screen.getByRole("status", { name: "Loading the recorded track" })).toBeInTheDocument();
  });

  // The three ways a ride has no line read differently to a rider: one is worth
  // waiting for, the others are not.
  it.each([
    ["pending" as const, "The ride's samples have not been read from Wahoo yet."],
    ["empty" as const, "This ride recorded too few positions to draw a line."],
    ["unreadable" as const, "The ride's recorded file could not be read."],
  ])("says why a %s ride has no line", (state, message) => {
    show({ coordinates: [], state });

    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.queryByTestId("activity-map")).not.toBeInTheDocument();
  });

  it("requests no track for a non-integer activity id", () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL) => new Response(null, { status: 500 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(null, "7.5");

    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("track"))).toBe(false);
    expect(screen.getByText("No recorded track was stored for this ride.")).toBeInTheDocument();
  });
});
