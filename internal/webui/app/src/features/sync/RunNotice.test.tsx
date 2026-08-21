import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Status, SyncPhaseRun, SyncRun } from "../../api/types";
import { noticeRun, RunNotice } from "./RunNotice";

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

function phaseRun(overrides: Partial<SyncPhaseRun> = {}): SyncPhaseRun {
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

function status(phases: Status["sync"]["phases"] = {}): Status {
  return {
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases,
      surface: { classified: 0, total: 0 },
    },
  };
}

describe("noticeRun", () => {
  /*
   * A notification names a run and the operator arrives hours later. That run is
   * what they were told about, even if it succeeded and something else has gone
   * wrong since: landing them on the newer trouble answers a question they did
   * not ask.
   */
  it("honours a reference whatever it says", () => {
    const named = run({ reference: "aaaaaaaaaaaa" });

    expect(
      noticeRun(
        "aaaaaaaaaaaa",
        [named],
        status({ targets: phaseRun({ lastResult: "failed", lastFailure: "destination" }) }),
      ),
    ).toBe(named);
  });

  // The service prunes old runs, so a notification can outlive what it pointed
  // at. That is an ordinary end, and the caller has to be able to tell.
  it("finds nothing for a reference the history no longer holds", () => {
    expect(noticeRun("aaaaaaaaaaaa", [run({ reference: "bbbbbbbbbbbb" })], status())).toBeNull();
  });

  it("falls back to a half whose last run still needs something", () => {
    const notice = noticeRun(
      null,
      [],
      status({
        targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
      }),
    );

    expect(notice).toMatchObject({
      phase: "targets",
      result: "blocked",
      failure: "deletion_limit",
    });
    // The status carries no reference; the matching row in the history has one.
    expect(notice?.reference).toBe("");
  });

  // A write over a library that failed to read is the next thing to go wrong,
  // so the read is the one to raise.
  it("prefers the read when both halves need something", () => {
    expect(
      noticeRun(
        null,
        [],
        status({
          source: phaseRun({ lastResult: "failed", lastFailure: "source" }),
          targets: phaseRun({ lastResult: "blocked", lastFailure: "deletion_limit" }),
        }),
      ),
    ).toMatchObject({ phase: "source" });
  });

  it("stays quiet when nothing needs attention and nothing was named", () => {
    expect(noticeRun(null, [run()], status({ targets: phaseRun() }))).toBeNull();
    expect(noticeRun(null, [], undefined)).toBeNull();
  });
});

function renderNotice(
  reference: string | null,
  runs: SyncRun[],
  value: Status = status(),
): QueryClient {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(["status"], value);
  client.setQueryData(["sync-run-lookup", reference ?? ""], {
    pages: [{ runs }],
    pageParams: [undefined],
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <RunNotice reference={reference} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

  return client;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("RunNotice", () => {
  it("names the run a notification was sent about, and what to do", () => {
    renderNotice("aaaaaaaaaaaa", [
      run({ reference: "aaaaaaaaaaaa", result: "blocked", failure: "deletion_limit" }),
    ]);

    expect(
      screen.getByRole("heading", {
        name: /removed more owned routes than the configured maximum/,
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/run aaaaaaaaaaaa/)).toBeInTheDocument();
    // The wire category never reaches the page; the words come from guidance.
    expect(screen.queryByText(/deletion_limit/)).not.toBeInTheDocument();
  });

  /*
   * Whatever a remediation asks for beyond running the half again is settled in
   * the service's configuration or in the cards below, so the second action
   * leaves rather than offering a control that cannot do it.
   */
  it("offers the run again and a way out, and nothing it cannot do", () => {
    renderNotice("aaaaaaaaaaaa", [
      run({ reference: "aaaaaaaaaaaa", result: "blocked", failure: "deletion_limit" }),
    ]);

    expect(screen.getByRole("button", { name: "Run the write again" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dismiss" })).toHaveAttribute("href", "/sync");
  });

  it("asks the service to run the half the notice is about", async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
    renderNotice("aaaaaaaaaaaa", [
      run({
        reference: "aaaaaaaaaaaa",
        phase: "source",
        result: "failed",
        failure: "source",
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Run the read again" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/v1/sync/source",
        expect.objectContaining({ method: "POST" }),
      ),
    );
  });

  it("says so when that run could not be started", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("a run is already under way", { status: 409 })),
    );
    renderNotice("aaaaaaaaaaaa", [
      run({ reference: "aaaaaaaaaaaa", result: "failed", failure: "destination" }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Run the write again" }));

    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  /*
   * The history is read ten runs at a time and a notification can be older than
   * the first page of it. The card walks back through the pages rather than
   * calling the run pruned because it was not in the first ten.
   */
  it("keeps asking for older pages until the named run turns up", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        return new Response(
          JSON.stringify({
            runs: [
              {
                reference: "aaaaaaaaaaaa",
                phase: "targets",
                completed_at: "2026-08-17T06:30:00Z",
                result: "succeeded",
                source_stages: 0,
                created: 1,
                updated: 0,
                deleted: 0,
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
        mutations: { retry: false },
      },
    });
    client.setQueryData(["status"], status());
    // One page held, and a cursor saying the history goes further back.
    client.setQueryData(["sync-run-lookup", "aaaaaaaaaaaa"], {
      pages: [{ runs: [run({ reference: "bbbbbbbbbbbb" })], next: "cursor" }],
      pageParams: [undefined],
    });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <RunNotice reference="aaaaaaaaaaaa" />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "That run finished" })).toBeInTheDocument();
    expect(screen.queryByText(/no longer kept/)).toBeNull();
  });

  /*
   * A history that could not be read says nothing about the run: reporting that
   * as a pruning would tell the operator their run is gone on the strength of a
   * lookup that never happened.
   */
  it("tells an unreadable history apart from a pruned run", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("no", { status: 500 })),
    );
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
        mutations: { retry: false },
      },
    });
    client.setQueryData(["status"], status());
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <RunNotice reference="aaaaaaaaaaaa" />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByRole("heading", { name: "That run could not be looked up" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/pruned/)).toBeNull();
  });

  // The notification outlived what it pointed at. That is the pruning working,
  // not a fault, and the page says which reference it could not find.
  it("says a named run has been pruned", () => {
    renderNotice("aaaaaaaaaaaa", [run({ reference: "bbbbbbbbbbbb" })]);

    expect(screen.getByRole("heading", { name: "That run is no longer kept" })).toBeInTheDocument();
    expect(screen.getByText(/aaaaaaaaaaaa/)).toBeInTheDocument();
  });

  it("shows nothing at all when nothing needs the operator", () => {
    renderNotice(null, [run()], status({ targets: phaseRun() }));

    expect(document.querySelector(".run-notice")).toBeNull();
  });
});
