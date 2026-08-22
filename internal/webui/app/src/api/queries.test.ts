import { afterEach, describe, expect, it, vi } from "vitest";
import { routeGeometryQuery, routeQuery, statusQuery } from "./queries";

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

  return refetchInterval({ state: { data: { sync: { active } } } } as Parameters<
    typeof refetchInterval
  >[0]);
}

/**
 * A cached stage is addressed by the provider that issued it as well as by its
 * route and stage order. Leaving the provider out of the key would let two
 * providers' stages that happen to share a route ID serve each other's cache
 * entry, which is the confusion the provider exists to prevent.
 */
describe("a stage's query key", () => {
  it("names the provider alongside the route and stage", () => {
    expect(routeQuery("veloplanner", 12, 1).queryKey).toEqual(["stage", "veloplanner", 12, 1]);
    expect(routeGeometryQuery("veloplanner", 12, 1).queryKey).toEqual([
      "stage-geometry",
      "veloplanner",
      12,
      1,
    ]);
  });

  it("separates the same route and stage under a different provider", () => {
    expect(routeQuery("komoot", 12, 1).queryKey).not.toEqual(
      routeQuery("veloplanner", 12, 1).queryKey,
    );
  });

  // The key and the request have to name the same stage. A query that keyed on
  // the provider but asked for a stage without it would cache one answer under
  // another's name.
  it("asks for the stage its key names", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            provider: "veloplanner",
            route_id: 12,
            stage: 1,
            title: "Kaiserstuhl Loop",
            route_name: "Kaiserstuhl Loop",
            stage_name: "",
            distance_metres: 1000,
            point_count: 2,
          }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { queryFn } = routeQuery("veloplanner", 12, 1);
    if (typeof queryFn !== "function") {
      throw new Error("the stage query no longer fetches for itself");
    }
    await queryFn({} as Parameters<typeof queryFn>[0]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/v1/providers/veloplanner/routes/12/stages/1",
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
