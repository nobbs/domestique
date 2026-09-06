import { afterEach, describe, expect, it, vi } from "vitest";
import {
  activitiesQuery,
  activityTrackQuery,
  routeGeometryQuery,
  routeQuery,
  statusQuery,
} from "./queries";
import type { Activity } from "./types";

afterEach(() => {
  vi.unstubAllGlobals();
});

/**
 * The interval is a function of the last answer, so what it decides is worth
 * asserting directly: a timer that kept asking after a run finished would be
 * asking on nobody's behalf, and one that stopped during a run would leave the
 * operator refreshing the page by hand — the thing the live line exists to
 * spare them.
 */
function intervalWhile(active: unknown): number | false | undefined {
  const { refetchInterval } = statusQuery();
  if (typeof refetchInterval !== "function") {
    throw new Error("the status query no longer decides its own interval");
  }

  return refetchInterval({ state: { data: { sync: { active } } } } as unknown as Parameters<
    typeof refetchInterval
  >[0]);
}

/**
 * A cached route is addressed by the provider that issued it as well as by its
 * source route and stage order. Leaving the provider out of the key would let
 * two providers' routes that share a source route ID serve each other's cache
 * entry, which is the confusion the provider exists to prevent.
 */
describe("a route's query key", () => {
  it("names the provider alongside the source route and stage order", () => {
    expect(routeQuery("veloplanner", 12, 1).queryKey).toEqual([
      "/v1/providers/veloplanner/sourceRoutes/12/routes/1",
    ]);
    expect(routeGeometryQuery("veloplanner", 12, 1).queryKey).toEqual([
      "/v1/providers/veloplanner/sourceRoutes/12/routes/1/geometry",
    ]);
  });

  // A track is a ride's own, so its key is the ride's id and nothing else.
  it("keys a recorded track by the activity it belongs to", () => {
    expect(activityTrackQuery(7).queryKey).toEqual(["/v1/activities/7/track"]);
  });

  // The altitudes travel beside the line on the wire, and the profile reads
  // them off the positions, so the query is where the two are put back together.
  it("folds a track's altitudes into its positions", () => {
    const { select } = activityTrackQuery(7);
    const track = select?.({
      status: 200,
      headers: new Headers(),
      data: {
        type: "Feature",
        bbox: [8.4, 49, 8.5, 49.1],
        geometry: {
          type: "LineString",
          coordinates: [
            [8.4, 49],
            [8.5, 49.1],
          ],
        },
        properties: { altitudeMetres: [110, 180] },
      },
    });

    expect(track?.coordinates).toEqual([
      [8.4, 49, 110],
      [8.5, 49.1, 180],
    ]);
    expect(track?.bbox).toEqual([8.4, 49, 8.5, 49.1]);
  });

  it("separates the same source route and stage order under a different provider", () => {
    expect(routeQuery("komoot", 12, 1).queryKey).not.toEqual(
      routeQuery("veloplanner", 12, 1).queryKey,
    );
  });

  // The key and the request have to name the same route. A query that keyed on
  // the provider but asked for a route without it would cache one answer under
  // another's name.
  it("asks for the route its key names", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            provider: "veloplanner",
            sourceRouteId: 12,
            stageOrder: 1,
            title: "Kaiserstuhl Loop",
            sourceRouteName: "Kaiserstuhl Loop",
            routeName: "",
            distanceMetres: 1000,
            pointCount: 2,
          }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { queryFn } = routeQuery("veloplanner", 12, 1);
    if (typeof queryFn !== "function") {
      throw new Error("the route query no longer fetches for itself");
    }
    await queryFn({} as Parameters<typeof queryFn>[0]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/providers/veloplanner/sourceRoutes/12/routes/1",
      expect.anything(),
    );
  });
});

describe("statusQuery", () => {
  it("polls while a run has not finished", () => {
    expect(intervalWhile({ targets: 1, stages: { current: 0, pending: 1 } })).toBe(2000);
  });

  it("stops polling once nothing is under way", () => {
    expect(intervalWhile(undefined)).toBe(false);
  });
});

/**
 * Seeded caches hold the list the page reads; a fetch answers with the wire
 * envelope around it. Both reach the same select.
 */
describe("the activities query", () => {
  it("reads the list out of the envelope, and leaves an unwrapped one alone", () => {
    const { select } = activitiesQuery();
    if (!select) {
      throw new Error("the activities query no longer selects");
    }
    type Response = Parameters<typeof select>[0];
    const activity = { id: 1 } as unknown as Activity;

    expect(select({ data: { activities: [activity] } } as unknown as Response)).toEqual([activity]);
    expect(select({ activities: [activity] } as unknown as Response)).toEqual([activity]);
    expect(select([activity] as unknown as Response)).toEqual([activity]);
  });
});
