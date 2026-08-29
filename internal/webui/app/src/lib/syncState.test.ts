import { describe, expect, it } from "vitest";
import type { Status, SyncStatus, TargetStatus } from "../api/types";
import { syncState } from "./syncState";

function status(sync: Partial<SyncStatus> = {}, targets: TargetStatus[] = []): Status {
  return {
    ready: true,
    converged: true,
    targets,
    sync: {
      state: "idle",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
      ...sync,
    },
  };
}

function unauthorized(): TargetStatus {
  return {
    id: "rider-b",
    authorisation: "not_authorized",
    convergence: "unauthorized",
    stages: { current: 0, pending: 4 },
  };
}

function phaseRun(overrides: Record<string, unknown> = {}) {
  return {
    lastCompletedAt: "2026-08-18T06:30:00Z",
    lastResult: "succeeded",
    sourceStages: 0,
    created: 0,
    updated: 0,
    deleted: 0,
    ...overrides,
  };
}

describe("syncState", () => {
  it("says what is happening while it happens", () => {
    expect(
      syncState(
        status({
          state: "running",
          active: { phase: "targets", targets: 1, stages: { current: 0, pending: 4 } },
        }),
      ),
    ).toEqual({ label: "Writing to Wahoo", tone: undefined });
  });

  // A run that has been accepted but has not picked a half yet is still work,
  // and a line that says nothing at all reads as a line that is out of date.
  it("says a run has started before it says which half", () => {
    expect(
      syncState(
        status({ state: "running", active: { targets: 1, stages: { current: 0, pending: 0 } } }),
      ).label,
    ).toBe("Starting");
  });

  it("separates a run being held back from a run under way", () => {
    expect(
      syncState(
        status({
          state: "delayed",
          active: {
            phase: "targets",
            startsAt: "2026-08-18T06:45:00Z",
            targets: 1,
            stages: { current: 0, pending: 4 },
          },
        }),
      ).label,
    ).toBe("Waiting to start");
  });

  /*
   * The whole point of the line: a gate that held is a decision waiting for the
   * operator, and a run that broke is a fault. They are painted differently
   * because they ask for different things.
   */
  it("tells a held gate apart from a fault", () => {
    const held = syncState(
      status({
        lastCompletedAt: "2026-08-18T06:30:00Z",
        phases: {
          targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
        },
      }),
    );
    const broken = syncState(
      status({
        lastCompletedAt: "2026-08-18T06:30:00Z",
        phases: { targets: phaseRun({ lastResult: "failed", lastFailure: "destination" }) },
      }),
    );

    expect(held.tone).toBe("hold");
    expect(held.label).toMatch(/^Held by a gate · /);
    expect(broken.tone).toBe("alert");
    expect(broken.label).toMatch(/^Did not finish · /);
  });

  // Reading comes first because a write over a stale library is the next thing
  // to go wrong, and one line has room for one of the two.
  it("names the read before the write when both need attention", () => {
    expect(
      syncState(
        status({
          phases: {
            source: phaseRun({ lastResult: "failed", lastFailure: "source" }),
            targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
          },
        }),
      ).label,
    ).toMatch(/^Did not finish · /);
  });

  /*
   * A half that never started is not a half that broke. `syncGuidance` speaks
   * for both, and reading anything it returns as a fault paints the ordinary
   * overlap of two schedules in the colour reserved for something being wrong.
   */
  it("does not read a run that never started as one that failed", () => {
    const skipped = syncState(
      status({
        lastCompletedAt: "2026-08-18T06:30:00Z",
        phases: { targets: phaseRun({ lastResult: "skipped" }) },
      }),
    );

    expect(skipped.tone).not.toBe("alert");
    expect(skipped.label).not.toMatch(/^Did not finish/);
  });

  // The same for a half held back until a target is connected: that is the
  // unconnected target, and the target is what the line should say.
  it("names the unconnected target rather than the run it held back", () => {
    expect(
      syncState(
        status(
          {
            lastCompletedAt: "2026-08-18T06:30:00Z",
            phases: { targets: phaseRun({ lastResult: "not_ready" }) },
          },
          [unauthorized()],
        ),
      ),
    ).toEqual({ label: "A target is not connected", tone: "alert" });
  });

  it("raises a target that cannot be written to at all", () => {
    expect(
      syncState(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }, [unauthorized()])),
    ).toEqual({ label: "A target is not connected", tone: "alert" });
  });

  it("says nothing has run rather than that everything is in sync", () => {
    expect(syncState(status())).toEqual({ label: "Has not run yet", tone: undefined });
  });

  /*
   * A library that has been read but not yet written everywhere is the ordinary
   * state between two runs. It is not green, and it is emphatically not red.
   */
  it("keeps a library that is merely behind unpainted", () => {
    const behind = syncState({
      ...status({ lastCompletedAt: "2026-08-18T06:30:00Z" }),
      converged: false,
    });

    expect(behind.tone).toBeUndefined();
    expect(behind.label).toMatch(/^Behind · /);
  });

  it("paints only a library every target holds", () => {
    const state = syncState(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }));

    expect(state.tone).toBe("good");
    expect(state.label).toMatch(/^In sync · /);
  });
});
