import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Status, TargetStatus } from "../../api/types";
import { TargetConvergence } from "./TargetConvergence";

function target(overrides: Partial<TargetStatus> = {}): TargetStatus {
  return {
    id: "rider-a",
    authorisation: "authorized",
    convergence: "current",
    stages: { current: 4, pending: 0 },
    lastRun: { completedAt: "2026-08-18T06:00:00Z", result: "succeeded" },
    ...overrides,
  };
}

function status(converged: boolean, targets: TargetStatus[]): Status {
  return {
    ready: true,
    converged,
    targets,
    sync: {
      state: "idle",
      sourceStages: 4,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0 },
    },
  };
}

function renderConvergence(value: Status) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(["status"], value);

  return render(
    <QueryClientProvider client={client}>
      <TargetConvergence />
    </QueryClientProvider>,
  );
}

describe("TargetConvergence", () => {
  it("reports each account's own state", () => {
    renderConvergence(
      status(false, [
        target(),
        target({
          id: "rider-b",
          convergence: "failed",
          stages: { current: 3, pending: 1 },
          lastRun: {
            completedAt: "2026-08-18T06:00:04Z",
            result: "failed",
            failure: "destination",
          },
        }),
      ]),
    );

    expect(screen.getByText("rider-a")).toBeInTheDocument();
    expect(screen.getByText("Up to date")).toBeInTheDocument();
    expect(screen.getByText("All 4 stages written.")).toBeInTheDocument();
    expect(screen.getByText("Last write failed")).toBeInTheDocument();
    expect(screen.getByText("3 written · 1 outstanding")).toBeInTheDocument();
    expect(screen.getByText(/failed \(destination\)/)).toBeInTheDocument();
  });

  // "Up to date" is a claim about the Wahoo account. Whether a head unit has
  // fetched those routes is not something the service can see, and the section
  // must not be readable as though it were.
  it("says this is the account and not the device", () => {
    renderConvergence(status(true, [target()]));

    expect(screen.getByText(/not what a head unit has downloaded/)).toBeInTheDocument();
    expect(screen.getByText(/Every stored stage is on every account/)).toBeInTheDocument();
  });

  it("says an account is waiting to be connected rather than merely behind", () => {
    renderConvergence(
      status(false, [
        target({
          authorisation: "not_authorized",
          convergence: "unauthorized",
          stages: { current: 0, pending: 4 },
          lastRun: undefined,
        }),
      ]),
    );

    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(screen.getByText(/Waiting to be connected to Wahoo/)).toBeInTheDocument();
    expect(screen.getByText("Has not been written to yet.")).toBeInTheDocument();
    expect(screen.getByText(/Some stored stages are not yet on every account/)).toBeInTheDocument();
  });
});
