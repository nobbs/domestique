import { describe, expect, it } from "vitest";
import {
  ContractError,
  parseRouteGeometry,
  parseRoutes,
  parseStatus,
  parseSyncRuns,
  parseWeather,
  parseWebUIConfig,
} from "./parse";

const stagePayload = {
  provider: "veloplanner",
  route_id: 12,
  stage: 1,
  title: "Alpine loop — Descent",
  route_name: "Alpine loop",
  stage_name: "Descent",
  source_revision: "2026-08-17",
  content_hash: "hash",
  distance_metres: 1234.5,
  point_count: 2,
};

describe("parseRoutes", () => {
  it("maps the wire contract onto domain values", () => {
    const stages = parseRoutes({ stages: [stagePayload] });

    expect(stages).toHaveLength(1);
    expect(stages[0]).toMatchObject({
      routeId: 12,
      stageOrder: 1,
      routeName: "Alpine loop",
      distanceMetres: 1234.5,
      pointCount: 2,
    });
  });

  it("accepts an empty library", () => {
    expect(parseRoutes({ stages: [] })).toEqual([]);
  });

  it("rejects a payload that drifts from the contract", () => {
    expect(() => parseRoutes({ stages: [{ ...stagePayload, route_id: "12" }] })).toThrow(
      ContractError,
    );
    expect(() => parseRoutes({})).toThrow(ContractError);
    expect(() => parseRoutes(null)).toThrow(ContractError);
  });

  it("leaves moving time and validation undefined when the service omits them", () => {
    const stages = parseRoutes({ stages: [stagePayload] });

    expect(stages[0]?.movingSeconds).toBeUndefined();
    expect(stages[0]?.validation).toBeUndefined();
  });

  it("reads a predicted moving time and its validation metadata when present", () => {
    const stages = parseRoutes({
      stages: [
        {
          ...stagePayload,
          moving_seconds: 4321.5,
          validation: {
            bias_percent: -1.2,
            mae_percent: 6.8,
            p90_percent: 14.1,
            evaluated_rides: 42,
          },
        },
      ],
    });

    expect(stages[0]?.movingSeconds).toBe(4321.5);
    expect(stages[0]?.validation).toEqual({
      biasPercent: -1.2,
      maePercent: 6.8,
      p90Percent: 14.1,
      evaluatedRides: 42,
    });
  });

  it("rejects a validation object missing one of its fields", () => {
    expect(() =>
      parseRoutes({
        stages: [
          {
            ...stagePayload,
            moving_seconds: 4321.5,
            validation: { bias_percent: -1.2, mae_percent: 6.8, p90_percent: 14.1 },
          },
        ],
      }),
    ).toThrow(ContractError);
  });

  it("rejects a moving time that is zero or negative", () => {
    expect(() => parseRoutes({ stages: [{ ...stagePayload, moving_seconds: 0 }] })).toThrow(
      ContractError,
    );
    expect(() => parseRoutes({ stages: [{ ...stagePayload, moving_seconds: -4 }] })).toThrow(
      ContractError,
    );
  });

  it("rejects an evaluated_rides count that is zero or not a whole number", () => {
    const withEvaluatedRides = (evaluatedRides: unknown) =>
      parseRoutes({
        stages: [
          {
            ...stagePayload,
            moving_seconds: 4321.5,
            validation: {
              bias_percent: -1.2,
              mae_percent: 6.8,
              p90_percent: 14.1,
              evaluated_rides: evaluatedRides,
            },
          },
        ],
      });

    expect(() => withEvaluatedRides(0)).toThrow(ContractError);
    expect(() => withEvaluatedRides(-3)).toThrow(ContractError);
    expect(() => withEvaluatedRides(4.5)).toThrow(ContractError);
  });

  it("rejects a negative mae_percent or p90_percent, but accepts a negative bias_percent", () => {
    const withValidation = (validation: Record<string, unknown>) =>
      parseRoutes({
        stages: [{ ...stagePayload, moving_seconds: 4321.5, validation }],
      });

    expect(() =>
      withValidation({
        bias_percent: -1.2,
        mae_percent: -0.1,
        p90_percent: 14.1,
        evaluated_rides: 42,
      }),
    ).toThrow(ContractError);
    expect(() =>
      withValidation({
        bias_percent: -1.2,
        mae_percent: 6.8,
        p90_percent: -0.1,
        evaluated_rides: 42,
      }),
    ).toThrow(ContractError);

    const stages = withValidation({
      bias_percent: -1.2,
      mae_percent: 6.8,
      p90_percent: 14.1,
      evaluated_rides: 42,
    });
    expect(stages[0]?.validation?.biasPercent).toBe(-1.2);
  });
});

describe("parseRouteGeometry", () => {
  const payload = {
    type: "Feature",
    bbox: [8.4, 49, 8.5, 49.2],
    geometry: {
      type: "LineString",
      coordinates: [
        [8.4, 49, 128.5],
        [8.5, 49.2],
      ],
    },
    properties: stagePayload,
  };

  it("reads coordinates with and without elevation", () => {
    const geometry = parseRouteGeometry(payload);

    expect(geometry.bbox).toEqual([8.4, 49, 8.5, 49.2]);
    expect(geometry.coordinates).toEqual([
      [8.4, 49, 128.5],
      [8.5, 49.2],
    ]);
    expect(geometry.stage.title).toBe("Alpine loop — Descent");
  });

  it("rejects a bounding box that is not four numbers", () => {
    expect(() => parseRouteGeometry({ ...payload, bbox: [8.4, 49] })).toThrow(ContractError);
  });

  it("rejects a position without a latitude", () => {
    expect(() =>
      parseRouteGeometry({
        ...payload,
        geometry: { type: "LineString", coordinates: [[8.4]] },
      }),
    ).toThrow(ContractError);
  });

  it("reads a surface classification beside the geometry it describes", () => {
    const geometry = parseRouteGeometry({
      ...payload,
      properties: {
        ...stagePayload,
        surface: {
          ranges: [{ kind: "gravel", start_index: 0, end_index: 1 }],
          matched_metres: 1200,
        },
      },
    });

    expect(geometry.surface).toEqual({
      ranges: [{ kind: "gravel", startIndex: 0, endIndex: 1 }],
      matchedMetres: 1200,
    });
  });

  it("leaves the surface absent on a stage nothing has classified yet", () => {
    expect(parseRouteGeometry(payload).surface).toBeUndefined();
  });

  it("reads predicted moving time beside the geometry it describes", () => {
    const geometry = parseRouteGeometry({
      ...payload,
      properties: { ...stagePayload, cumulative_seconds: [0, 42.5] },
    });

    expect(geometry.cumulativeSeconds).toEqual([0, 42.5]);
  });

  it("leaves the moving time absent on a stage nothing has predicted yet", () => {
    expect(parseRouteGeometry(payload).cumulativeSeconds).toBeUndefined();
  });

  it("rejects a moving time series that drifts from the contract", () => {
    expect(() =>
      parseRouteGeometry({
        ...payload,
        properties: { ...stagePayload, cumulative_seconds: [0, "42.5"] },
      }),
    ).toThrow(ContractError);
  });

  it("rejects a moving time series that names a different number of points than the geometry", () => {
    expect(() =>
      parseRouteGeometry({
        ...payload,
        properties: { ...stagePayload, cumulative_seconds: [0] },
      }),
    ).toThrow(ContractError);
    expect(() =>
      parseRouteGeometry({
        ...payload,
        properties: { ...stagePayload, cumulative_seconds: [] },
      }),
    ).toThrow(ContractError);
  });

  it("keeps a classification that matched nothing distinct from an absent one", () => {
    const geometry = parseRouteGeometry({
      ...payload,
      properties: { ...stagePayload, surface: { ranges: [], matched_metres: 0 } },
    });

    expect(geometry.surface).toEqual({ ranges: [], matchedMetres: 0 });
  });

  it("degrades a class this build has never heard of to unknown", () => {
    const geometry = parseRouteGeometry({
      ...payload,
      properties: {
        ...stagePayload,
        surface: {
          ranges: [{ kind: "quicksand", start_index: 0, end_index: 1 }],
          matched_metres: 10,
        },
      },
    });

    expect(geometry.surface?.ranges[0]?.kind).toBe("unknown");
  });

  it("rejects a range index that addresses no point", () => {
    const withRange = (range: unknown) => ({
      ...payload,
      properties: { ...stagePayload, surface: { ranges: [range], matched_metres: 10 } },
    });

    expect(() =>
      parseRouteGeometry(withRange({ kind: "asphalt", start_index: -1, end_index: 1 })),
    ).toThrow(ContractError);
    expect(() =>
      parseRouteGeometry(withRange({ kind: "asphalt", start_index: 0, end_index: 1.5 })),
    ).toThrow(ContractError);
  });

  it("rejects a surface group that drifts from the contract", () => {
    const withSurface = (surface: unknown) => ({
      ...payload,
      properties: { ...stagePayload, surface },
    });

    expect(() => parseRouteGeometry(withSurface({ matched_metres: 10 }))).toThrow(ContractError);
    expect(() => parseRouteGeometry(withSurface({ ranges: [] }))).toThrow(ContractError);
    expect(() =>
      parseRouteGeometry(
        withSurface({ ranges: [{ kind: 3, start_index: 0, end_index: 1 }], matched_metres: 10 }),
      ),
    ).toThrow(ContractError);
    expect(() =>
      parseRouteGeometry(
        withSurface({
          ranges: [{ kind: "asphalt", start_index: 4, end_index: 1 }],
          matched_metres: 10,
        }),
      ),
    ).toThrow(ContractError);
  });
});

describe("parseStatus", () => {
  it("reads readiness and the last run", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [
        {
          id: "rider-a",
          authorisation: "authorized",
          convergence: "lagging",
          stages: { current: 3, pending: 1 },
          last_run: { completed_at: "2026-08-17T08:00:00Z", result: "succeeded" },
        },
      ],
      sync: {
        state: "succeeded",
        last_result: "succeeded",
        last_completed_at: "2026-08-17T08:00:00Z",
        source_stages: 4,
        created: 1,
        updated: 2,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.ready).toBe(true);
    expect(status.targets[0]?.id).toBe("rider-a");
    expect(status.sync.sourceStages).toBe(4);
    expect(status.sync.lastCompletedAt).toBe("2026-08-17T08:00:00Z");
    // Absent until the service has actually talked to Wahoo.
    expect(status.sync.wahooRateLimit).toBeUndefined();
  });

  // Wahoo's own live reading, not a count this service totals itself.
  it("reads the observed Wahoo quota", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
        wahoo_rate_limit: { remaining: 187, resets_at: "2026-08-23T12:00:00Z" },
      },
    });

    expect(status.sync.wahooRateLimit).toEqual({
      remaining: 187,
      resetsAt: "2026-08-23T12:00:00Z",
    });
  });

  it("reads each account's convergence, counts, and last write", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [
        {
          id: "rider-a",
          authorisation: "authorized",
          convergence: "current",
          stages: { current: 4, pending: 0 },
          last_run: { completed_at: "2026-08-18T06:00:00Z", result: "succeeded" },
        },
        {
          id: "rider-b",
          authorisation: "authorized",
          convergence: "failed",
          stages: { current: 3, pending: 1 },
          last_run: {
            completed_at: "2026-08-18T06:00:04Z",
            result: "failed",
            failure: "destination",
          },
        },
      ],
      sync: {
        state: "failed",
        source_stages: 4,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.converged).toBe(false);
    expect(status.targets[0]?.convergence).toBe("current");
    expect(status.targets[0]?.stages).toEqual({ current: 4, pending: 0 });
    expect(status.targets[0]?.lastRun?.failure).toBeUndefined();
    expect(status.targets[1]?.convergence).toBe("failed");
    expect(status.targets[1]?.stages.pending).toBe(1);
    expect(status.targets[1]?.lastRun?.failure).toBe("destination");
  });

  // An account that has never been written to is absent from the run record, not
  // an account whose write succeeded with nothing to do.
  it("reads an account that has never been written to", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [
        {
          id: "rider-a",
          authorisation: "not_authorized",
          convergence: "unauthorized",
          stages: { current: 0, pending: 4 },
        },
      ],
      sync: {
        state: "not_ready",
        source_stages: 4,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.targets[0]?.lastRun).toBeUndefined();
    expect(status.targets[0]?.convergence).toBe("unauthorized");
  });

  // A run that has not finished is the one thing on this page that is not a
  // record of something that already happened.
  it("reads the run that has not finished", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [],
      sync: {
        state: "running",
        active: {
          phase: "targets",
          starts_at: "2026-08-18T06:05:00Z",
          targets: 2,
          stages: { current: 11, pending: 1 },
        },
        source_stages: 12,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.sync.state).toBe("running");
    expect(status.sync.active?.phase).toBe("targets");
    expect(status.sync.active?.startsAt).toBe("2026-08-18T06:05:00Z");
    expect(status.sync.active?.targets).toBe(2);
    expect(status.sync.active?.stages).toEqual({ current: 11, pending: 1 });
  });

  // Absent is the answer that nothing is happening, so it must not become an
  // empty run the page then has to explain.
  it("reads no run under way as none", () => {
    const status = parseStatus({
      ready: true,
      converged: true,
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.sync.active).toBeUndefined();
  });

  // A half this build has never heard of leaves the run without a name for what
  // it is doing, which is still better than presenting the word as one.
  it("reads an unfamiliar half of a running sync as no half", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [],
      sync: {
        state: "running",
        active: { phase: "reticulating", targets: 1, stages: { current: 0, pending: 0 } },
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.sync.active?.phase).toBeUndefined();
    expect(status.sync.active?.targets).toBe(1);
  });

  // A word this build has never heard of cannot be presented as "everything is
  // fine", so it degrades to the state that asks the operator to look.
  it("degrades an unfamiliar convergence to failed", () => {
    const status = parseStatus({
      ready: true,
      converged: false,
      targets: [
        {
          id: "rider-a",
          authorisation: "authorized",
          convergence: "reticulating",
          stages: { current: 0, pending: 0 },
        },
      ],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.targets[0]?.convergence).toBe("failed");
  });

  it("refuses a status missing overall convergence or an account's counts", () => {
    const sync = {
      state: "idle",
      source_stages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
    };

    expect(() => parseStatus({ ready: true, targets: [], sync })).toThrow(ContractError);
    expect(() =>
      parseStatus({
        ready: true,
        converged: true,
        targets: [{ id: "rider-a", authorisation: "authorized", convergence: "current" }],
        sync,
      }),
    ).toThrow(ContractError);
  });

  it("reads the build the service is running", () => {
    const revision = "0123456789abcdef0123456789abcdef01234567";
    const status = parseStatus({
      ready: true,
      converged: true,
      targets: [],
      build: { revision, image_digest: `sha256:${"cd".repeat(32)}` },
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.build?.revision).toBe(revision);
    expect(status.build?.imageDigest).toBe(`sha256:${"cd".repeat(32)}`);
  });

  // A build stamp is not worth the status page: the service drops a value it
  // would not stand behind, so an absent or identity-less group is read as "not
  // built by CI" rather than as a broken contract.
  it("treats a missing or identity-less build as no build", () => {
    const withBuild = (build: unknown) => ({
      ready: true,
      converged: true,
      targets: [],
      ...(build === undefined ? {} : { build }),
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(parseStatus(withBuild(undefined)).build).toBeUndefined();
    expect(parseStatus(withBuild(null)).build).toBeUndefined();
    expect(parseStatus(withBuild({})).build).toBeUndefined();
    expect(parseStatus(withBuild({ revision: "" })).build).toBeUndefined();
    expect(
      parseStatus(withBuild({ image_digest: `sha256:${"cd".repeat(32)}` })).build,
    ).toBeUndefined();
  });

  it("tolerates a run that has never completed", () => {
    const status = parseStatus({
      ready: false,
      converged: true,
      targets: [],
      sync: {
        state: "not_ready",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(status.sync.lastCompletedAt).toBeUndefined();
  });

  it("reads both schedule switches and each half's last run", () => {
    const status = parseStatus({
      ready: true,
      converged: true,
      targets: [],
      sync: {
        state: "failed",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        surface: { classified: 1, total: 3, incomplete: 0 },
        schedule: { source: true, targets: false },
        phases: {
          source: {
            last_completed_at: "2026-08-18T06:00:00Z",
            last_result: "succeeded",
            source_stages: 12,
            created: 0,
            updated: 0,
            deleted: 0,
          },
          targets: {
            last_completed_at: "2026-08-18T06:00:04Z",
            last_result: "failed",
            last_failure: "destination",
            source_stages: 12,
            created: 1,
            updated: 0,
            deleted: 0,
          },
        },
      },
    });

    expect(status.sync.schedule).toEqual({ source: true, targets: false });
    expect(status.sync.phases.source?.sourceStages).toBe(12);
    expect(status.sync.phases.targets?.lastFailure).toBe("destination");
  });

  // A half is absent until it has finished a run, which is not the same as one
  // that finished badly.
  it("leaves a half absent until it has run", () => {
    const status = parseStatus({
      ready: true,
      converged: true,
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        surface: { classified: 3, total: 3, incomplete: 0 },
        schedule: { source: true, targets: true },
        phases: {
          source: {
            last_completed_at: "2026-08-18T06:00:00Z",
            last_result: "succeeded",
            source_stages: 12,
            created: 0,
            updated: 0,
            deleted: 0,
          },
        },
      },
    });

    expect(status.sync.phases.source).toBeDefined();
    expect(status.sync.phases.targets).toBeUndefined();
  });

  // A missing switch is not an off switch: rendering a control from an assumed
  // value would show a state the service never reported.
  it("refuses a schedule that names only one switch", () => {
    const withSchedule = (schedule: unknown) => ({
      ready: true,
      converged: true,
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule,
        phases: {},
        surface: { classified: 0, total: 0, incomplete: 0 },
      },
    });

    expect(() => parseStatus(withSchedule({ source: true }))).toThrow(ContractError);
    expect(() => parseStatus(withSchedule({ source: true, targets: "yes" }))).toThrow(
      ContractError,
    );
  });
});

describe("parseSyncRuns", () => {
  const runPayload = {
    reference: "1a2b3c4d5e6f",
    phase: "targets",
    completed_at: "2026-08-18T06:30:00Z",
    result: "failed",
    failure: "destination",
    source_stages: 0,
    created: 1,
    updated: 0,
    deleted: 0,
  };

  it("reads a page and the cursor for the one after it", () => {
    const page = parseSyncRuns({ runs: [runPayload], next: "412" });

    expect(page.next).toBe("412");
    expect(page.runs[0]).toEqual({
      reference: "1a2b3c4d5e6f",
      phase: "targets",
      completedAt: "2026-08-18T06:30:00Z",
      result: "failed",
      failure: "destination",
      sourceStages: 0,
      created: 1,
      updated: 0,
      deleted: 0,
    });
  });

  it("reads the last page as one with no cursor after it", () => {
    expect(parseSyncRuns({ runs: [] }).next).toBeUndefined();
  });

  // Every row is labelled by its half, so a half this build cannot name would
  // reach the page as a run of nothing in particular.
  it("refuses a run whose half it cannot name", () => {
    expect(() => parseSyncRuns({ runs: [{ ...runPayload, phase: "surface" }] })).toThrow(
      ContractError,
    );
  });

  it("refuses a run recorded without the counts it is measured by", () => {
    const { created: _created, ...withoutCreated } = runPayload;

    expect(() => parseSyncRuns({ runs: [withoutCreated] })).toThrow(ContractError);
  });
});

describe("parseWebUIConfig", () => {
  const streets = {
    name: "Streets",
    style_url: "https://tiles.example.test/styles/bright",
    style_url_dark: "https://tiles.example.test/styles/dark",
  };
  const satellite = {
    name: "Satellite",
    style_url: "https://imagery.example.test/maps/hybrid/style.json",
    dark_cartography: true,
  };

  it("reads the basemaps in the order they are offered", () => {
    const config = parseWebUIConfig({ basemaps: [streets, satellite] });

    expect(config.basemaps).toEqual([
      {
        name: "Streets",
        styleUrl: "https://tiles.example.test/styles/bright",
        styleUrlDark: "https://tiles.example.test/styles/dark",
        darkCartography: false,
      },
      {
        name: "Satellite",
        styleUrl: "https://imagery.example.test/maps/hybrid/style.json",
        styleUrlDark: undefined,
        darkCartography: true,
      },
    ]);
  });

  it("reads a deployment that names no source provider", () => {
    expect(parseWebUIConfig({ basemaps: [streets] }).sourceBaseUrls).toEqual({});
  });

  it("reads a base URL per configured source, keyed by provider", () => {
    const config = parseWebUIConfig({
      basemaps: [streets],
      source_base_urls: { veloplanner: "https://veloplanner.example" },
    });

    expect(config.sourceBaseUrls).toEqual({ veloplanner: "https://veloplanner.example" });
  });

  it("refuses a source base URL entry that is not a string", () => {
    expect(() =>
      parseWebUIConfig({ basemaps: [streets], source_base_urls: { veloplanner: 4 } }),
    ).toThrow(ContractError);
  });

  // The service refuses an empty list at startup, so one on the wire is a build
  // talking to something that is not this service. Better a named contract
  // error than a map that silently loads nothing.
  it("refuses a configuration offering no basemap", () => {
    expect(() => parseWebUIConfig({ basemaps: [] })).toThrow(ContractError);
  });

  it("refuses a basemap with no name to pick it by", () => {
    const { name: _name, ...unnamed } = streets;

    expect(() => parseWebUIConfig({ basemaps: [unnamed] })).toThrow(ContractError);
  });

  it("refuses a basemap with no style to load", () => {
    expect(() => parseWebUIConfig({ basemaps: [{ name: "Streets" }] })).toThrow(ContractError);
  });

  it("refuses a darkness marker that is not a boolean", () => {
    expect(() =>
      parseWebUIConfig({ basemaps: [{ ...satellite, dark_cartography: "yes" }] }),
    ).toThrow(ContractError);
  });

  it("refuses a basemap named only blank space", () => {
    expect(() => parseWebUIConfig({ basemaps: [{ ...streets, name: "  " }] })).toThrow(
      ContractError,
    );
  });

  it("refuses two basemaps sharing one name", () => {
    expect(() =>
      parseWebUIConfig({ basemaps: [streets, { ...satellite, name: streets.name }] }),
    ).toThrow(ContractError);
  });

  it("refuses a basemap that is dark cartography with a dark style of its own", () => {
    expect(() =>
      parseWebUIConfig({
        basemaps: [
          { ...satellite, style_url_dark: "https://imagery.example.test/maps/hybrid/dark.json" },
        ],
      }),
    ).toThrow(ContractError);
  });
});

describe("parseWeather", () => {
  const pointPayload = {
    time: "2026-08-25T09:00:00Z",
    temperature_celsius: 18.2,
    apparent_temperature_celsius: 17.1,
    precipitation_millimetres: 0.4,
    precipitation_probability_percent: 35,
    wind_speed_kmh: 18,
    wind_direction_degrees: 240,
    weather_code: 61,
  };

  it("reads a forecast point into the domain shape", () => {
    const forecast = parseWeather({ points: [pointPayload] });

    expect(forecast.points).toEqual([
      {
        time: "2026-08-25T09:00:00Z",
        temperatureCelsius: 18.2,
        apparentTemperatureCelsius: 17.1,
        precipitationMillimetres: 0.4,
        precipitationProbabilityPercent: 35,
        windSpeedKmh: 18,
        windDirectionDegrees: 240,
        weatherCode: 61,
      },
    ]);
  });

  it("accepts an empty forecast", () => {
    expect(parseWeather({ points: [] }).points).toEqual([]);
  });

  it("refuses a point missing one of its measurements", () => {
    const { wind_speed_kmh: _windSpeedKmh, ...withoutWindSpeed } = pointPayload;

    expect(() => parseWeather({ points: [withoutWindSpeed] })).toThrow(ContractError);
  });

  it("refuses a point whose weather code is not a number", () => {
    expect(() => parseWeather({ points: [{ ...pointPayload, weather_code: "clear" }] })).toThrow(
      ContractError,
    );
  });
});
