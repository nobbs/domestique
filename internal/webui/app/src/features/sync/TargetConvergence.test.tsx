import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Status, TargetStatus } from "../../api/types";
import { TargetConvergence } from "./TargetConvergence";

afterEach(() => {
  vi.unstubAllGlobals();
});

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
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
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
    expect(screen.getByText(/^All 4 routes · written /)).toBeInTheDocument();
    expect(screen.getByText(/^3 of 4 routes · written /)).toBeInTheDocument();
    // The wire words are gone: an operator is told what happened and what to do,
    // not handed the category the notification would have carried.
    expect(screen.queryByText(/failed \(destination\)/)).not.toBeInTheDocument();
    expect(screen.getByText(/Did not finish/)).toBeInTheDocument();
    expect(screen.getByText(/a Wahoo operation did not complete/)).toBeInTheDocument();
  });

  // "All 4 routes" is a claim about the Wahoo account. Whether a head unit has
  // fetched those routes is not something the service can see, and the card
  // must not be readable as though it were.
  it("says this is the account and not the device", () => {
    renderConvergence(status(true, [target()]));

    expect(screen.getByText(/not what a head unit has downloaded/)).toBeInTheDocument();
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
    expect(screen.getByText(/has never been connected to Wahoo/)).toBeInTheDocument();
    // The counts are not repeated beside it: an account that cannot be written
    // to at all has nothing to say about how far behind it is.
    expect(screen.queryByText(/routes/)).not.toBeInTheDocument();
  });

  /*
   * The point of the whole section, for an operator who has just deployed: the
   * flow that fixes an unconnected account is a redirect, and until now the
   * page named no way to it at all.
   */
  it.each([
    ["not_authorized", "Connect rider-a"],
    ["needs_reauthorization", "Reconnect rider-a"],
  ])("offers the protected flow for a %s account", (authorisation, name) => {
    renderConvergence(
      status(false, [
        target({ authorisation, convergence: "unauthorized", stages: { current: 0, pending: 4 } }),
      ]),
    );

    // The slot is in the link text because two accounts sit in this list.
    const connect = screen.getByRole("link", { name });
    // The service's own protected route, reached by leaving the application:
    // the flow goes to Wahoo and comes back, so it cannot be a client-side one.
    expect(connect).toHaveAttribute("href", "/oauth/wahoo/start/rider-a");
  });

  it("separates an account Wahoo rejected from one never connected", () => {
    renderConvergence(
      status(false, [
        target({
          authorisation: "needs_reauthorization",
          convergence: "unauthorized",
          stages: { current: 4, pending: 0 },
          lastRun: {
            completedAt: "2026-08-18T06:00:04Z",
            result: "failed",
            failure: "authorization",
          },
        }),
      ]),
    );

    expect(screen.getByText("Reconnect needed")).toBeInTheDocument();
    expect(screen.getByText(/Wahoo stopped accepting/)).toBeInTheDocument();
    // Nothing about a rejected authorisation costs the operator any data, and a
    // page that leaves that unsaid invites a panicked re-run.
    expect(screen.getByText(/Nothing was deleted and nothing has been lost/)).toBeInTheDocument();
  });

  /*
   * A second flow invalidates the first, so an account midway through
   * connecting must be told to finish the one it has rather than handed a
   * control that quietly destroys it.
   */
  it("offers nothing to press while a connection is in flight", () => {
    renderConvergence(
      status(false, [
        target({
          authorisation: "pending",
          convergence: "unauthorized",
          stages: { current: 0, pending: 4 },
          lastRun: undefined,
        }),
      ]),
    );

    expect(screen.getByText("Connecting")).toBeInTheDocument();
    expect(screen.getByText(/A connection was started and has not come back/)).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByText("Not connected")).not.toBeInTheDocument();
  });

  it("asks the operator to look when it does not recognise the state", () => {
    renderConvergence(
      status(false, [
        target({ authorisation: "revoked_by_operator", convergence: "unauthorized" }),
      ]),
    );

    expect(screen.getByText("Connection state unknown")).toBeInTheDocument();
    expect(screen.getByText(/may be older than the service/)).toBeInTheDocument();
    // Starting a flow against a state this build cannot explain is a guess, and
    // one that would invalidate whatever flow is already running.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  /*
   * The service reduces every unsuccessful run to `failed` in its one word, so
   * an account held by a deletion gate arrives here looking exactly like an
   * account that broke. It is the opposite situation: nothing was removed, and
   * running it again changes nothing until the operator decides it should.
   */
  it.each([
    ["deletion_limit", /more owned routes than the configured maximum/],
    ["empty_source", /came back empty after previously holding stages/],
  ])("reads a %s gate as held rather than as a fault", (failure, explanation) => {
    renderConvergence(
      status(false, [
        target({
          convergence: "failed",
          stages: { current: 4, pending: 2 },
          lastRun: { completedAt: "2026-08-18T06:00:04Z", result: "blocked", failure },
        }),
      ]),
    );

    expect(screen.getByText(/^Held by a safety gate · /)).toBeInTheDocument();
    expect(screen.queryByText(/^Did not finish · /)).not.toBeInTheDocument();
    expect(screen.getByText(explanation)).toBeInTheDocument();
    expect(screen.getByText(/Nothing was deleted/)).toBeInTheDocument();
  });

  it("keeps a genuine fault reading as one", () => {
    renderConvergence(
      status(false, [
        target({
          convergence: "failed",
          lastRun: { completedAt: "2026-08-18T06:00:04Z", result: "failed", failure: "state" },
        }),
      ]),
    );

    expect(screen.getByText(/^Did not finish · /)).toBeInTheDocument();
    expect(screen.queryByText(/^Held by a safety gate · /)).not.toBeInTheDocument();
  });

  // The action names the account it reconciles, not a physical device: an
  // operator reading two rows must be able to tell which one a press affects.
  it("offers to reconcile a connected account by name", () => {
    renderConvergence(status(true, [target(), target({ id: "rider-b" })]));

    expect(screen.getByRole("button", { name: "Reconcile now: rider-a" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reconcile now: rider-b" })).toBeInTheDocument();
  });

  // An account that cannot be written to has nothing here worth reconciling:
  // the connect or reconnect flow is the only action offered instead.
  it.each(["not_authorized", "pending", "needs_reauthorization"])(
    "offers no reconcile action for a %s account",
    (authorisation) => {
      renderConvergence(status(false, [target({ authorisation, convergence: "unauthorized" })]));

      expect(screen.queryByRole("button", { name: /^Reconcile now/ })).not.toBeInTheDocument();
    },
  );

  it("triggers exactly the pressed account's reconciliation", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ status: "accepted" }), { status: 202 });
      }

      return new Response(JSON.stringify(status(true, [target(), target({ id: "rider-b" })])), {
        status: 200,
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    renderConvergence(status(true, [target(), target({ id: "rider-b" })]));

    await userEvent.click(screen.getByRole("button", { name: "Reconcile now: rider-b" }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.some((call) => call[0] === "/v1/sync/targets/rider-b")).toBe(
        true,
      ),
    );
    expect(fetchMock.mock.calls.some((call) => call[0] === "/v1/sync/targets/rider-a")).toBe(false);
  });

  // A refused reconciliation is worth showing: the operator pressed something
  // and nothing happened.
  it("reports a rejected reconciliation", async () => {
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
    renderConvergence(status(true, [target()]));

    await userEvent.click(screen.getByRole("button", { name: "Reconcile now: rider-a" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "a synchronization is already running",
    );
  });
});
