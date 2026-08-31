import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import type { Status, TaskList, WebUIConfig } from "../../api/types";
import { activeSummary, idleSummary, SyncControls } from "./SyncControls";

function status(overrides: Partial<Status["sync"]> = {}): Status {
  return {
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      sourceRoutes: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
      ...overrides,
    },
  };
}

function config(sourceBaseUrls: Record<string, string> = { veloplanner: "https://v.example" }) {
  const value: WebUIConfig = {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style", darkCartography: false },
    ],
    sourceBaseUrls,
    identity: { email: "rider@example.test" },
  };

  return value;
}

// tasks is the registered list a test seeds, with each half's switch where the
// page now reads it.
function tasks(source = true, targets = true): TaskList {
  return {
    tasks: [
      { name: "sync:source", scheduled: true, enabled: source, running: 0 },
      { name: "sync:target", scheduled: true, enabled: targets, running: 0 },
    ],
  };
}

function renderControls(
  value: Status = status(),
  configValue: WebUIConfig = config(),
  taskList: TaskList = tasks(),
) {
  // The seeded status and config are the whole fixture: without this the
  // queries refetch on mount and every assertion below would have to step
  // over a /v1/status or /v1/webui/config call.
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(statusQuery().queryKey, value);
  client.setQueryData(webUIConfigQuery().queryKey, configValue);
  client.setQueryData(tasksQuery().queryKey, taskList);

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
  // The whole point of the line: a run under way says so, rather than leaving
  // the last finished run on the page as though it were the answer.
  it("says which half is running and how much the targets already hold", () => {
    renderControls(
      status({
        state: "running",
        active: { phase: "targets", targets: 2, routes: { current: 11, pending: 1 } },
      }),
    );

    expect(
      screen.getByText("Writing to Wahoo · 11 of 12 routes across 2 targets"),
    ).toBeInTheDocument();
  });

  // Accepted and not yet in either half. It is a moment long, and it is the
  // moment the operator who pressed the button is looking at.
  it("says a run has been accepted before either half starts", () => {
    renderControls(
      status({ state: "queued", active: { targets: 1, routes: { current: 0, pending: 0 } } }),
    );

    expect(screen.getByText("Starting")).toBeInTheDocument();
  });

  // A service holding its first run back has something to say for itself, and
  // "idle" is not it.
  it("says when a first run is being held back", () => {
    renderControls(
      status({
        state: "delayed",
        active: {
          startsAt: "2026-08-18T06:05:00Z",
          targets: 1,
          routes: { current: 3, pending: 1 },
        },
      }),
    );

    expect(
      screen.getByText(/^First run at .* · 3 of 4 routes across 1 target$/),
    ).toBeInTheDocument();
  });

  it("says nothing about a run when nothing is under way", () => {
    renderControls();

    expect(screen.queryByText(/Reading from VeloPlanner|Writing to Wahoo|Starting/)).toBeNull();
  });

  it("shows each half with its own switch and button", () => {
    renderControls();

    expect(screen.getByText("Read from VeloPlanner")).toBeInTheDocument();
    expect(screen.getByText("Write to Wahoo")).toBeInTheDocument();
    // Each control names its own half: the visible words are the same in both
    // rows, so the accessible name is what tells them apart.
    expect(
      screen.getByRole("switch", { name: "Hourly: Read from VeloPlanner" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Hourly: Write to Wahoo" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run now: Read from VeloPlanner" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run now: Write to Wahoo" })).toBeInTheDocument();
  });

  // A single configured source is still named, because that stays the
  // friendlier answer while it is simple.
  it("reads as the library rather than naming any source when none is configured", () => {
    renderControls(status(), config({}));

    expect(screen.getByText("Read from the source library")).toBeInTheDocument();
  });

  // More than one source configured reads the same generic way: naming one of
  // two would say less than the truth.
  it("reads as the library rather than naming one source when more than one is configured", () => {
    renderControls(
      status(),
      config({ veloplanner: "https://v.example", komoot: "https://k.example" }),
    );

    expect(screen.getByText("Read from the source library")).toBeInTheDocument();
  });

  // The service refuses a half-named schedule, so a change to one switch has to
  // carry the operator's view of the other.
  it("sends both switches when one is changed", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify(input === "/v1/status" ? status() : tasks(true, false))),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderControls(status(), config(), tasks(true, true));

    await userEvent.click(screen.getByRole("switch", { name: "Hourly: Write to Wahoo" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((call) => call[0] === "/v1/tasks/sync%3Atarget/schedule"),
      ).toBe(true),
    );
    const scheduleCall = fetchMock.mock.calls.find(
      (call) => call[0] === "/v1/tasks/sync%3Atarget/schedule",
    );
    // One switch travels alone: nothing here carries a value for the other half.
    expect(scheduleCall?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ enabled: false }),
    });
  });

  // The switch governs the timer; the button is the operator, who has already
  // decided.
  it("still runs a half whose schedule is switched off", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify(input === "/v1/status" ? status() : { status: "accepted" })),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderControls(status(), config(), tasks(false, false));

    const sourceButton = screen.getByRole("button", { name: /Read from VeloPlanner/ });
    expect(sourceButton).toBeEnabled();
    await userEvent.click(sourceButton);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/sync%3Asource/run", expect.anything()),
    );
  });

  // Both switches carry the same two words, because the interval the service
  // runs on is fixed at an hour. Which of them is on is the checkbox itself.
  it("shows the state of each switch on the switch", () => {
    renderControls(status(), config(), tasks(true, false));

    expect(screen.getByRole("switch", { name: "Hourly: Read from VeloPlanner" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.getByRole("switch", { name: "Hourly: Write to Wahoo" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  it("summarises each half's last run in its own terms", () => {
    renderControls(
      status({
        phases: {
          source: {
            lastCompletedAt: "2026-08-18T06:00:00Z",
            lastResult: "succeeded",
            sourceRoutes: 12,
            created: 0,
            updated: 0,
            deleted: 0,
          },
          targets: {
            lastCompletedAt: "2026-08-18T06:00:04Z",
            lastResult: "failed",
            lastFailure: "destination",
            sourceRoutes: 12,
            created: 1,
            updated: 0,
            deleted: 0,
          },
        },
      }),
    );

    expect(screen.getByText(/12 routes/)).toBeInTheDocument();
    expect(screen.queryByText(/failed \(destination\)/)).not.toBeInTheDocument();
    expect(screen.getByText(/did not finish$/)).toBeInTheDocument();
    expect(screen.getByText(/Writing to Wahoo could not finish/)).toBeInTheDocument();
  });

  /*
   * The two gates, in the half that trips them. Nothing was deleted in either
   * case, and the way past both is a deliberate configuration change rather than
   * pressing the button again — which is exactly what an operator does when a
   * gate is presented as a failure.
   */
  it.each([
    ["deletion_limit", /raise the per-run deletion maximum deliberately/],
    ["empty_source", /empty-source deletion acknowledgement/],
  ])("explains the %s gate and what would clear it", (lastFailure, remediation) => {
    renderControls(
      status({
        phases: {
          targets: {
            lastCompletedAt: "2026-08-18T06:00:04Z",
            lastResult: "blocked",
            lastFailure,
            sourceRoutes: 12,
            created: 0,
            updated: 0,
            deleted: 0,
          },
        },
      }),
    );

    expect(screen.getByText(/held by a gate$/)).toBeInTheDocument();
    expect(screen.getByText(/Writing to Wahoo stopped/)).toBeInTheDocument();
    expect(screen.getByText(remediation)).toBeInTheDocument();
  });

  it("names the half a source failure happened in", () => {
    renderControls(
      status({
        phases: {
          source: {
            lastCompletedAt: "2026-08-18T06:00:00Z",
            lastResult: "failed",
            lastFailure: "source",
            sourceRoutes: 0,
            created: 0,
            updated: 0,
            deleted: 0,
          },
        },
      }),
    );

    expect(screen.getByText(/Reading the library could not finish/)).toBeInTheDocument();
    // The non-destructive half of the promise is the part worth stating: an
    // incomplete read must never be read as routes having gone away.
    expect(screen.getByText(/Nothing was deleted/)).toBeInTheDocument();
  });

  it("says a half has not run rather than showing an empty result", () => {
    renderControls();

    expect(screen.getAllByText("Has not run yet")).toHaveLength(2);
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

  // Classification never fails a run, so a route the endpoint keeps refusing is
  // otherwise indistinguishable from one that has not come up yet.
  it("says how much of the library is still unclassified", () => {
    renderControls(status({ surface: { classified: 1, total: 3, incomplete: 0 } }));

    expect(screen.getByText(/classified for 1 of 3 routes/)).toBeInTheDocument();
  });

  it("counts one route as a route", () => {
    renderControls(status({ surface: { classified: 0, total: 1, incomplete: 0 } }));

    expect(screen.getByText(/classified for 0 of 1 route\./)).toBeInTheDocument();
  });

  it("says nothing about surfaces once the whole library is classified", () => {
    renderControls(status({ surface: { classified: 3, total: 3, incomplete: 0 } }));

    expect(screen.queryByText(/classified for/)).not.toBeInTheDocument();
  });

  // A route that keeps failing classification is otherwise indistinguishable
  // from one that has not come up yet — the count and the retry are the two
  // places that difference shows.
  it("offers a retry once a route could not be classified", () => {
    renderControls(status({ surface: { classified: 1, total: 3, incomplete: 1 } }));

    expect(screen.getByText(/1 could not be classified last time/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry now" })).toBeInTheDocument();
  });

  it("offers no retry while nothing has failed to classify", () => {
    renderControls(status({ surface: { classified: 1, total: 3, incomplete: 0 } }));

    expect(screen.queryByRole("button", { name: "Retry now" })).not.toBeInTheDocument();
  });

  it("retries classification without touching either sync phase", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(
          JSON.stringify(
            input === "/v1/status"
              ? status({ surface: { classified: 1, total: 3, incomplete: 1 } })
              : { status: "accepted" },
          ),
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderControls(status({ surface: { classified: 1, total: 3, incomplete: 1 } }));

    await userEvent.click(screen.getByRole("button", { name: "Retry now" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/v1/tasks/surface%3Aannotate/run",
        expect.objectContaining({ method: "POST" }),
      ),
    );
  });

  it("says what went wrong when the retry is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response(
            JSON.stringify({
              error: { code: "sync_in_progress", message: "a synchronization is already running" },
            }),
            { status: 409 },
          ),
      ),
    );
    renderControls(status({ surface: { classified: 1, total: 3, incomplete: 1 } }));

    await userEvent.click(screen.getByRole("button", { name: "Retry now" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "a synchronization is already running",
    );
  });
});

describe("activeSummary", () => {
  // The instant is what makes a delay worth reporting, but a service that did
  // not give one is still waiting rather than idle.
  it("says a held-back run is waiting when nothing said until when", () => {
    expect(activeSummary("delayed", { targets: 1, routes: { current: 1, pending: 1 } })).toBe(
      "Waiting to start · 1 of 2 routes across 1 target",
    );
  });
});

describe("idleSummary", () => {
  it("says when each half last got somewhere", () => {
    expect(
      idleSummary(
        status({
          phases: {
            source: {
              lastCompletedAt: "2026-08-18T06:00:00Z",
              lastResult: "succeeded",
              sourceRoutes: 12,
              created: 0,
              updated: 0,
              deleted: 0,
            },
          },
        }),
      ),
    ).toMatch(/^Nothing is running\. Last read .*, last write .*\.$/);
  });

  // A gate that held is the one thing on the card an operator has to act on, so
  // it replaces the ordinary two clauses rather than sitting beside them.
  it("says a held gate instead of when the half finished", () => {
    expect(
      idleSummary(
        status({
          phases: {
            targets: {
              lastCompletedAt: "2026-08-18T06:00:04Z",
              lastResult: "blocked",
              lastFailure: "deletion_limit",
              sourceRoutes: 12,
              created: 0,
              updated: 0,
              deleted: 0,
            },
          },
        }),
      ),
    ).toMatch(/^Nothing is running\. The last write was held at .*\.$/);
  });

  it("says nothing has run rather than naming two absent times", () => {
    expect(idleSummary(status())).toBe("Nothing is running, and nothing has run yet.");
  });
});
