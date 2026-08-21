import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Status, SyncStatus, TargetStatus } from "../api/types";
import { Wordmark, wordmarkState } from "./Wordmark";

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
      surface: { classified: 0, total: 0 },
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

describe("wordmarkState", () => {
  it("says what is happening while it happens", () => {
    expect(
      wordmarkState(
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
      wordmarkState(
        status({ state: "running", active: { targets: 1, stages: { current: 0, pending: 0 } } }),
      ).label,
    ).toBe("Starting");
  });

  it("separates a run being held back from a run under way", () => {
    expect(
      wordmarkState(
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
    const held = wordmarkState(
      status({
        lastCompletedAt: "2026-08-18T06:30:00Z",
        phases: {
          targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
        },
      }),
    );
    const broken = wordmarkState(
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
      wordmarkState(
        status({
          phases: {
            source: phaseRun({ lastResult: "failed", lastFailure: "source" }),
            targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
          },
        }),
      ).label,
    ).toMatch(/^Did not finish · /);
  });

  it("raises an account that cannot be written to at all", () => {
    expect(
      wordmarkState(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }, [unauthorized()])),
    ).toEqual({ label: "An account is not connected", tone: "alert" });
  });

  it("says nothing has run rather than that everything is in sync", () => {
    expect(wordmarkState(status())).toEqual({ label: "Has not run yet", tone: undefined });
  });

  /*
   * A library that has been read but not yet written everywhere is the ordinary
   * state between two runs. It is not green, and it is emphatically not red.
   */
  it("keeps a library that is merely behind unpainted", () => {
    const behind = wordmarkState({
      ...status({ lastCompletedAt: "2026-08-18T06:30:00Z" }),
      converged: false,
    });

    expect(behind.tone).toBeUndefined();
    expect(behind.label).toMatch(/^Behind · /);
  });

  it("paints only a library every account holds", () => {
    const state = wordmarkState(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }));

    expect(state.tone).toBe("good");
    expect(state.label).toMatch(/^In sync · /);
  });
});

function renderWordmark(value?: Status) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (value) {
    client.setQueryData(["status"], value);
  }

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Wordmark />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Wordmark", () => {
  it("carries one quiet way to the sync page", () => {
    renderWordmark(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }));

    const link = screen.getByRole("link", { name: /^Sync/ });
    expect(link).toHaveAttribute("href", "/sync");
    expect(link).toHaveTextContent("Sync");
    expect(screen.getByText("domestique")).toBeInTheDocument();
  });

  /*
   * The row has room for one word, so the state is the link's colour — and a
   * colour is nothing to a screen reader or to anyone who cannot tell these two
   * apart. The name says what the colour meant.
   */
  it("says in the link's name what its colour means", () => {
    renderWordmark(status({ lastCompletedAt: "2026-08-18T06:30:00Z" }, [unauthorized()]));

    const link = screen.getByRole("link", { name: "Sync \u00b7 An account is not connected" });
    expect(link).toHaveAttribute("data-tone", "alert");
  });

  /*
   * The map behind this panel is what the reader came for. A status request
   * still in flight — or one that never arrives — must not paint the corner of
   * it in a state nobody has.
   */
  it("says nothing about a state it does not have", () => {
    renderWordmark();

    const link = screen.getByRole("link", { name: "Sync" });
    expect(link).not.toHaveAttribute("data-tone");
    expect(link).not.toHaveAttribute("title");
  });
});
