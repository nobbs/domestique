import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, syncRunsQueryKey, webUIConfigQuery } from "../../api/queries";
import type { Status, SyncRun, SyncRunPage, WebUIConfig } from "../../api/types";
import { SyncHistory } from "./SyncHistory";

function status(lastCompletedAt: string): Status {
  return {
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      lastCompletedAt,
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
    },
  };
}

function run(overrides: Partial<SyncRun> = {}): SyncRun {
  return {
    reference: "1a2b3c4d5e6f",
    phase: "targets",
    completedAt: "2026-08-18T06:30:00Z",
    result: "succeeded",
    sourceStages: 0,
    created: 1,
    updated: 2,
    deleted: 0,
    ...overrides,
  };
}

function config(sourceBaseUrls: Record<string, string> = { veloplanner: "https://v.example" }) {
  const value: WebUIConfig = {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style", darkCartography: false },
    ],
    sourceBaseUrls,
  };

  return value;
}

function renderHistory(page: SyncRunPage, configValue: WebUIConfig = config()) {
  // The seeded page is the whole fixture: without it the query fetches on mount
  // and every assertion below would have to step over a /v1/sync/runs call.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(syncRunsQueryKey(), { pages: [page], pageParams: [undefined] });
  client.setQueryData(statusQuery().queryKey, status("2026-08-18T06:30:00Z"));
  client.setQueryData(webUIConfigQuery().queryKey, configValue);
  render(
    <QueryClientProvider client={client}>
      <SyncHistory />
    </QueryClientProvider>,
  );

  return client;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SyncHistory", () => {
  it("shows each recorded run by half, counts, outcome, and reference", () => {
    renderHistory({
      runs: [
        run({ reference: "aaaaaaaaaaaa" }),
        run({ reference: "bbbbbbbbbbbb", phase: "source", sourceStages: 12 }),
      ],
    });

    expect(screen.getByText("Write to Wahoo")).toBeInTheDocument();
    expect(screen.getByText("1 created · 2 updated")).toBeInTheDocument();
    expect(screen.getByText("Read from VeloPlanner")).toBeInTheDocument();
    expect(screen.getByText("12 routes")).toBeInTheDocument();
    expect(screen.getAllByText("Succeeded")).toHaveLength(2);
    // The reference is what a Pushover message carries, so it must be here to
    // be matched against; nothing else on the row identifies the run.
    expect(screen.getByText("aaaaaaaaaaaa")).toBeInTheDocument();
    expect(screen.getByText("bbbbbbbbbbbb")).toBeInTheDocument();
  });

  // Naming one of several configured sources would say less than the truth, so
  // more than one reads as the library instead.
  it("reads a source row as the library when more than one source is configured", () => {
    renderHistory(
      { runs: [run({ reference: "bbbbbbbbbbbb", phase: "source", sourceStages: 12 })] },
      config({ veloplanner: "https://v.example", komoot: "https://k.example" }),
    );

    expect(screen.getByText("Read from the source library")).toBeInTheDocument();
  });

  // A binary rolled back past the migration that named runs records rows with
  // no reference, and two of them are still two runs.
  it("shows a run recorded before runs were named", () => {
    renderHistory({
      runs: [
        run({ reference: "", completedAt: "2026-08-18T06:30:00Z" }),
        run({ reference: "", completedAt: "2026-08-18T06:00:00Z" }),
      ],
    });

    expect(screen.getAllByText("1 created · 2 updated")).toHaveLength(2);
    expect(screen.queryByText("aaaaaaaaaaaa")).toBeNull();
    expect(screen.queryByText("bbbbbbbbbbbb")).toBeNull();
  });

  // A gate that held and a run that broke are opposite events, and the raw
  // category says neither.
  it("tells a held gate apart from a fault", () => {
    renderHistory({
      runs: [
        run({ reference: "aaaaaaaaaaaa", result: "blocked", failure: "deletion_limit" }),
        run({ reference: "bbbbbbbbbbbb", result: "failed", failure: "destination" }),
      ],
    });

    expect(screen.getByText("Held by a safety gate")).toBeInTheDocument();
    expect(screen.getByText("Did not finish")).toBeInTheDocument();
    expect(screen.queryByText(/deletion_limit/)).not.toBeInTheDocument();
    expect(screen.queryByText(/destination/)).not.toBeInTheDocument();
  });

  it("follows the service's cursor for the runs before the page", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            runs: [
              {
                reference: "cccccccccccc",
                phase: "source",
                completed_at: "2026-08-18T04:30:00Z",
                result: "succeeded",
                source_stages: 9,
                created: 0,
                updated: 0,
                deleted: 0,
              },
            ],
          }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderHistory({ runs: [run({ reference: "aaaaaaaaaaaa" })], next: "412" });

    await userEvent.click(screen.getByRole("button", { name: "Earlier runs" }));

    await waitFor(() => expect(screen.getByText("cccccccccccc")).toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith("/v1/sync/runs?limit=10&after=412", expect.anything());
    // The page that came back ends the history, so there is nothing left to ask
    // for and the control that would ask goes away.
    expect(screen.queryByRole("button", { name: "Earlier runs" })).not.toBeInTheDocument();
  });

  // The list is one row behind the moment a run finishes, and the status beside
  // it is already being polled while that run is in flight.
  it("re-reads the history when a run finishes", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ runs: [] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const client = renderHistory({ runs: [run()] });
    expect(fetchMock).not.toHaveBeenCalled();

    act(() => {
      client.setQueryData(statusQuery().queryKey, status("2026-08-18T07:30:00Z"));
    });

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/sync/runs?limit=10", expect.anything()),
    );
  });

  it("says so when nothing has run yet", () => {
    renderHistory({ runs: [] });

    expect(screen.getByText("Nothing has run yet.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Earlier runs" })).not.toBeInTheDocument();
  });
});
