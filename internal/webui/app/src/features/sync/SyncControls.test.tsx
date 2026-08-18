import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Status } from "../../api/types";
import { SyncControls } from "./SyncControls";

function status(overrides: Partial<Status["sync"]> = {}): Status {
  return {
    ready: true,
    targets: [],
    sync: {
      state: "idle",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0 },
      ...overrides,
    },
  };
}

function renderControls(value: Status = status()) {
  // The seeded status is the whole fixture: without this the query refetches on
  // mount and every assertion below would have to step over a /v1/status call.
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(["status"], value);

  return render(
    <QueryClientProvider client={client}>
      <SyncControls />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SyncControls", () => {
  it("shows each half with its own switch and button", () => {
    renderControls();

    expect(screen.getByText("Read from VeloPlanner")).toBeInTheDocument();
    expect(screen.getByText("Write to Wahoo")).toBeInTheDocument();
    // Each control names its own half: the visible words are the same in both
    // rows, so the accessible name is what tells them apart.
    expect(
      screen.getByRole("checkbox", { name: "Schedule: Read from VeloPlanner" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Schedule: Write to Wahoo" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run now: Read from VeloPlanner" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run now: Write to Wahoo" })).toBeInTheDocument();
  });

  // The service refuses a half-named schedule, so a change to one switch has to
  // carry the operator's view of the other.
  it("sends both switches when one is changed", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ source: true, targets: false })),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderControls(status({ schedule: { source: true, targets: true } }));

    await userEvent.click(screen.getByRole("checkbox", { name: "Schedule: Write to Wahoo" }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.some((call) => call[0] === "/v1/sync/schedule")).toBe(true),
    );
    const scheduleCall = fetchMock.mock.calls.find((call) => call[0] === "/v1/sync/schedule");
    expect(scheduleCall?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ source: true, targets: false }),
    });
  });

  // The switch governs the timer; the button is the operator, who has already
  // decided.
  it("still runs a half whose schedule is switched off", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" })),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderControls(status({ schedule: { source: false, targets: false } }));

    const sourceButton = screen.getByRole("button", { name: /Read from VeloPlanner/ });
    expect(sourceButton).toBeEnabled();
    await userEvent.click(sourceButton);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/sync/source", expect.anything()),
    );
  });

  it("names the state of each switch", () => {
    renderControls(status({ schedule: { source: true, targets: false } }));

    expect(screen.getByText("Scheduled")).toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
  });

  it("summarises each half's last run in its own terms", () => {
    renderControls(
      status({
        phases: {
          source: {
            lastCompletedAt: "2026-08-18T06:00:00Z",
            lastResult: "succeeded",
            sourceStages: 12,
            created: 0,
            updated: 0,
            deleted: 0,
          },
          targets: {
            lastCompletedAt: "2026-08-18T06:00:04Z",
            lastResult: "failed",
            lastFailure: "destination",
            sourceStages: 12,
            created: 1,
            updated: 0,
            deleted: 0,
          },
        },
      }),
    );

    expect(screen.getByText(/12 stages/)).toBeInTheDocument();
    expect(screen.getByText(/failed \(destination\)/)).toBeInTheDocument();
  });

  it("says a half has not run rather than showing an empty result", () => {
    renderControls();

    expect(screen.getAllByText("Has not run yet.")).toHaveLength(2);
  });

  // A refused run is worth showing: the operator pressed a button and nothing
  // happened.
  it("reports a rejected run", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: { code: "sync_in_progress", message: "a synchronization is already running" },
            }),
            { status: 409 },
          ),
      ),
    );
    renderControls();

    await userEvent.click(screen.getByRole("button", { name: "Run now: Read from VeloPlanner" }));

    // An error the operator caused by pressing something is announced, not
    // queued behind whatever else a screen reader is saying.
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "a synchronization is already running",
    );
  });

  // Classification never fails a run, so a stage the endpoint keeps refusing is
  // otherwise indistinguishable from one that has not come up yet.
  it("says how much of the library is still unclassified", () => {
    renderControls(status({ surface: { classified: 1, total: 3 } }));

    expect(screen.getByText(/classified for 1 of 3 stages/)).toBeInTheDocument();
  });

  it("says nothing about surfaces once the whole library is classified", () => {
    renderControls(status({ surface: { classified: 3, total: 3 } }));

    expect(screen.queryByText(/classified for/)).not.toBeInTheDocument();
  });
});
