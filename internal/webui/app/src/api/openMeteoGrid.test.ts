import { describe, expect, it, vi } from "vitest";

const latestResponse = vi.hoisted(() => ({
  value: Promise.resolve({
    data: { reference_time: "2026-09-05T12:00:00Z", valid_times: ["2026-09-05T12:00Z"] },
    status: 200,
  }) as Promise<unknown>,
}));
const getWeatherGridLatest = vi.hoisted(() => vi.fn(() => latestResponse.value));

vi.mock("./generated", () => ({
  getWeatherGridLatest,
  getGetWeatherGridObjectUrl: (params: { referenceTime: string; validTime: string }) =>
    `/v1/weather-grid/object?referenceTime=${encodeURIComponent(params.referenceTime)}&validTime=${encodeURIComponent(params.validTime)}`,
}));

const seenOmUrls = vi.hoisted(() => ({ urls: [] as string[] }));

vi.mock("@openmeteo/file-reader", () => ({
  OmDataType: { FloatArray: "FloatArray" },
  LruBlockCache: class {},
  OmHttpBackendPool: class {
    async withReader<T>(
      url: string,
      _cache: unknown,
      fn: (root: unknown) => Promise<T>,
    ): Promise<T> {
      seenOmUrls.urls.push(url);
      const variable = {
        read: async () => new Float32Array([1, 2, 3, 4]),
        dispose: () => {},
      };

      return fn({ getChildByName: async () => variable });
    }
  },
}));

const { fetchWindGrid, gridWindow, nearestValidTime, omUrl } = await import("./openMeteoGrid");

describe("omUrl", () => {
  it("asks the service's own relay, not the provider, with full-precision timestamps", () => {
    expect(omUrl(new Date("2026-09-05T12:00:00Z"), "2026-09-05T15:00Z")).toBe(
      "/v1/weather-grid/object?referenceTime=2026-09-05T12%3A00%3A00.000Z&validTime=2026-09-05T15%3A00%3A00.000Z",
    );
  });

  it("normalises a valid time that omits seconds, which Go's RFC3339 parser refuses outright", () => {
    const url = omUrl(new Date("2026-01-02T03:00:00Z"), "2026-01-02T03:00Z");
    expect(url).toContain("validTime=2026-01-02T03%3A00%3A00.000Z");
  });
});

describe("nearestValidTime", () => {
  const times = ["2026-09-05T12:00Z", "2026-09-05T13:00Z", "2026-09-05T15:00Z"];

  it("picks the published hour closest to the one asked for", () => {
    expect(
      nearestValidTime(times[0] as string, times.slice(1), new Date("2026-09-05T13:40:00Z")),
    ).toBe("2026-09-05T13:00Z");
  });

  it("returns the only hour offered when there is nothing to compare it against", () => {
    expect(nearestValidTime("2026-09-05T12:00Z", [], new Date("2026-09-06T00:00:00Z"))).toBe(
      "2026-09-05T12:00Z",
    );
  });
});

describe("gridWindow", () => {
  it("floors the low edge and ceils the high edge to the file's 32-cell chunks", () => {
    expect(gridWindow([7.0, 48.0, 8.0, 49.0])).toEqual({ x0: 544, x1: 608, y0: 224, y1: 320 });
  });

  it("clamps a bbox that overruns the model's own domain to it", () => {
    expect(gridWindow([-100, -100, 100, 100])).toEqual({ x0: 0, x1: 1215, y0: 0, y1: 746 });
  });

  it("collapses to an empty window for a bbox entirely outside the domain", () => {
    expect(gridWindow([50, 60, 51, 61])).toEqual({ x0: 1215, x1: 1215, y0: 746, y1: 746 });
  });
});

describe("fetchWindGrid", () => {
  it("reads the manifest out of getWeatherGridLatest's response envelope, not the envelope itself", async () => {
    // domestiqueRequest wraps every generated call in { data, status, headers
    // }: a caller that read reference_time/valid_times straight off that
    // envelope would see undefined and build the .om request from garbage.
    seenOmUrls.urls.length = 0;
    const grid = await fetchWindGrid([7, 48, 8, 49], new Date("2026-09-05T12:00:00Z"));

    expect(grid).not.toBeNull();
    const [omUrlSeen] = seenOmUrls.urls;
    expect(omUrlSeen).toContain("referenceTime=2026-09-05T12%3A00%3A00.000Z");
    expect(omUrlSeen).toContain("validTime=2026-09-05T12%3A00%3A00.000Z");
  });

  it("does not let a stale in-flight manifest fetch evict a fresher one it lost the race to", async () => {
    // Comfortably past any cache the earlier tests in this file left behind,
    // so this test's first call is guaranteed to see it as expired too.
    const realNow = Date.now() + 600_000;
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(realNow);
    const manifest = (referenceTime: string) => ({
      data: { reference_time: referenceTime, valid_times: [`${referenceTime.slice(0, 16)}Z`] },
      status: 200,
    });
    let rejectStale: (reason: unknown) => void = () => {};
    getWeatherGridLatest.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectStale = reject;
      }),
    );

    // The stale request that will fail late, once something fresher has
    // already taken its place in the cache.
    const staleFetch = fetchWindGrid([7, 48, 8, 49], new Date()).catch(() => "rejected");
    nowSpy.mockReturnValue(realNow + 61_000); // past the manifest TTL

    getWeatherGridLatest.mockReturnValueOnce(Promise.resolve(manifest("2026-09-05T13:00:00Z")));
    const freshGrid = await fetchWindGrid([7, 48, 8, 49], new Date());
    expect(freshGrid).not.toBeNull();

    rejectStale(new Error("stale request failed late"));
    expect(await staleFetch).toBe("rejected");

    getWeatherGridLatest.mockClear();
    await fetchWindGrid([7, 48, 8, 49], new Date());
    expect(getWeatherGridLatest).not.toHaveBeenCalled();

    nowSpy.mockRestore();
  });
});
