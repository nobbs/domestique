import { describe, expect, it } from "vitest";
import { ContractError, parseStageGeometry, parseStages, parseStatus } from "./parse";

const stagePayload = {
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

describe("parseStages", () => {
  it("maps the wire contract onto domain values", () => {
    const stages = parseStages({ stages: [stagePayload] });

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
    expect(parseStages({ stages: [] })).toEqual([]);
  });

  it("rejects a payload that drifts from the contract", () => {
    expect(() => parseStages({ stages: [{ ...stagePayload, route_id: "12" }] })).toThrow(
      ContractError,
    );
    expect(() => parseStages({})).toThrow(ContractError);
    expect(() => parseStages(null)).toThrow(ContractError);
  });
});

describe("parseStageGeometry", () => {
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
    const geometry = parseStageGeometry(payload);

    expect(geometry.bbox).toEqual([8.4, 49, 8.5, 49.2]);
    expect(geometry.coordinates).toEqual([
      [8.4, 49, 128.5],
      [8.5, 49.2],
    ]);
    expect(geometry.stage.title).toBe("Alpine loop — Descent");
  });

  it("rejects a bounding box that is not four numbers", () => {
    expect(() => parseStageGeometry({ ...payload, bbox: [8.4, 49] })).toThrow(ContractError);
  });

  it("rejects a position without a latitude", () => {
    expect(() =>
      parseStageGeometry({
        ...payload,
        geometry: { type: "LineString", coordinates: [[8.4]] },
      }),
    ).toThrow(ContractError);
  });

  it("reads a surface classification beside the geometry it describes", () => {
    const geometry = parseStageGeometry({
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
    expect(parseStageGeometry(payload).surface).toBeUndefined();
  });

  it("keeps a classification that matched nothing distinct from an absent one", () => {
    const geometry = parseStageGeometry({
      ...payload,
      properties: { ...stagePayload, surface: { ranges: [], matched_metres: 0 } },
    });

    expect(geometry.surface).toEqual({ ranges: [], matchedMetres: 0 });
  });

  it("degrades a class this build has never heard of to unknown", () => {
    const geometry = parseStageGeometry({
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
      parseStageGeometry(withRange({ kind: "asphalt", start_index: -1, end_index: 1 })),
    ).toThrow(ContractError);
    expect(() =>
      parseStageGeometry(withRange({ kind: "asphalt", start_index: 0, end_index: 1.5 })),
    ).toThrow(ContractError);
  });

  it("rejects a surface group that drifts from the contract", () => {
    const withSurface = (surface: unknown) => ({
      ...payload,
      properties: { ...stagePayload, surface },
    });

    expect(() => parseStageGeometry(withSurface({ matched_metres: 10 }))).toThrow(ContractError);
    expect(() => parseStageGeometry(withSurface({ ranges: [] }))).toThrow(ContractError);
    expect(() =>
      parseStageGeometry(
        withSurface({ ranges: [{ kind: 3, start_index: 0, end_index: 1 }], matched_metres: 10 }),
      ),
    ).toThrow(ContractError);
    expect(() =>
      parseStageGeometry(
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
      targets: [{ id: "rider-a", authorisation: "authorized" }],
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
      },
    });

    expect(status.ready).toBe(true);
    expect(status.targets[0]?.id).toBe("rider-a");
    expect(status.sync.sourceStages).toBe(4);
    expect(status.sync.lastCompletedAt).toBe("2026-08-17T08:00:00Z");
  });

  it("tolerates a run that has never completed", () => {
    const status = parseStatus({
      ready: false,
      targets: [],
      sync: {
        state: "not_ready",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule: { source: true, targets: true },
        phases: {},
      },
    });

    expect(status.sync.lastCompletedAt).toBeUndefined();
  });

  it("reads both schedule switches and each half's last run", () => {
    const status = parseStatus({
      ready: true,
      targets: [],
      sync: {
        state: "failed",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
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
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
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
      targets: [],
      sync: {
        state: "idle",
        source_stages: 0,
        created: 0,
        updated: 0,
        deleted: 0,
        schedule,
        phases: {},
      },
    });

    expect(() => parseStatus(withSchedule({ source: true }))).toThrow(ContractError);
    expect(() => parseStatus(withSchedule({ source: true, targets: "yes" }))).toThrow(
      ContractError,
    );
  });
});
