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
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { activitiesQuery, activityTrackQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, ActivityTrack, Position, WebUIConfig } from "../../api/types";
import type { Profile } from "../../lib/profile";
import { ActivityPage } from "./ActivityPage";

const ZONE = "Europe/Berlin";

const drawn = vi.hoisted(() => ({
  coordinates: [] as Position[],
  bounds: null as number[] | null,
  profiles: [] as Array<Profile | null>,
}));

vi.mock("./ActivityMap", () => ({
  ActivityMap: (props: { coordinates: Position[]; bounds: number[]; profile: Profile | null }) => {
    drawn.coordinates = props.coordinates;
    drawn.bounds = props.bounds;

    return <div data-testid="activity-map" />;
  },
}));

vi.mock("../routes/ElevationProfile", () => ({
  ElevationProfile: (props: { profile: Profile | null }) => {
    drawn.profiles.push(props.profile);

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

  return { bbox: [8.4, 49, 8.6, 49.2], coordinates: positions };
}

function config(): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: ZONE,
    identity: { display: "rider@example.test", admin: false },
  };
}

function show(recorded: ActivityTrack | null = track(), activityId: number | string = RIDE.id) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config());
  client.setQueryData(activitiesQuery().queryKey, [RIDE]);
  if (recorded) {
    client.setQueryData(activityTrackQuery(RIDE.id).queryKey, recorded);
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

beforeEach(() => {
  drawn.coordinates = [];
  drawn.bounds = null;
  drawn.profiles = [];
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("one ride's page", () => {
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

  it("says plainly when no track was stored", () => {
    show({ bbox: [8, 49, 8, 49], coordinates: [] });

    expect(screen.getByText("No recorded track was stored for this ride.")).toBeInTheDocument();
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
