import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { taskRunsQueryKey, tasksQuery } from "../../../api/queries";
import type { TaskList, TaskRun, TaskRunPage } from "../../../api/types";
import { TaskRunFeed } from "./TaskRunFeed";

function tasks(): TaskList {
  return {
    tasks: [
      { name: "sync:source", scheduled: true, enabled: true, running: 0, intervalSeconds: 3600 },
      { name: "sync:clear", scheduled: false, enabled: true, running: 0 },
    ],
  };
}

function run(overrides: Partial<TaskRun> = {}): TaskRun {
  return {
    task: "sync:source",
    startedAt: "2026-08-31T06:00:00Z",
    finishedAt: "2026-08-31T06:00:04Z",
    outcome: "succeeded",
    reference: "aaaaaaaaaaaa",
    ...overrides,
  };
}

/** Reports the address the page is on, so a filter change can be read back. */
function Address() {
  const location = useLocation();

  return <span data-testid="address">{`${location.pathname}${location.search}`}</span>;
}

function renderFeed(page: TaskRunPage, entry = "/settings/tasks", taskList: TaskList = tasks()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  const url = new URL(entry, "https://example.test");
  client.setQueryData(taskRunsQueryKey(url.searchParams.get("task") ?? undefined), {
    pages: [page],
    pageParams: [undefined],
  });
  client.setQueryData(tasksQuery().queryKey, taskList);

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Address />
        <TaskRunFeed />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TaskRunFeed", () => {
  it("shows each recorded run's task, outcome, and reference", () => {
    renderFeed({
      runs: [
        run({ reference: "aaaaaaaaaaaa" }),
        run({ reference: "bbbbbbbbbbbb", task: "sync:clear", argument: "rider-a" }),
      ],
    });

    const list = within(screen.getByRole("list"));
    expect(list.getByText(/sync:source/)).toBeInTheDocument();
    expect(list.getByText(/sync:clear/)).toBeInTheDocument();
    expect(list.getByText("aaaaaaaaaaaa")).toBeInTheDocument();
    expect(list.getByText("bbbbbbbbbbbb")).toBeInTheDocument();
    expect(list.getAllByText("Succeeded")).toHaveLength(2);
  });

  it("says nothing has run yet", () => {
    renderFeed({ runs: [] });

    expect(screen.getByText("Nothing has run yet.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Earlier runs" })).not.toBeInTheDocument();
  });

  it("follows the service's cursor for the runs before the page", async () => {
    // Built through the same helper as the seeded page, so the fetched body
    // carries the contract's own field names rather than a second guess at them.
    const earlier = run({
      reference: "cccccccccccc",
      startedAt: "2026-08-31T04:00:00Z",
      finishedAt: "2026-08-31T04:00:03Z",
    });
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ runs: [earlier] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderFeed({ runs: [run({ reference: "aaaaaaaaaaaa" })], next: "412" });

    await userEvent.click(screen.getByRole("button", { name: "Earlier runs" }));

    await waitFor(() => expect(screen.getByText("cccccccccccc")).toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/runs?limit=10&after=412", expect.anything());
    // A run whose timestamp did not survive the wire reads as "never", so this
    // fails if a field is renamed out from under the page.
    expect(screen.queryByText("never")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Earlier runs" })).not.toBeInTheDocument();
  });

  it("reads the task filter from the address on load", () => {
    renderFeed({ runs: [run()] }, "/settings/tasks?task=sync%3Asource");

    expect(screen.getByRole("combobox", { name: "Task" })).toHaveValue("sync:source");
  });

  it("filters to one task and carries the choice into the address", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ runs: [] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderFeed({ runs: [run()] });

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Task" }), "sync:clear");

    await waitFor(() =>
      expect(screen.getByTestId("address")).toHaveTextContent("task=sync%3Aclear"),
    );
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/v1/tasks/runs?task=sync%3Aclear&limit=10",
        expect.anything(),
      ),
    );
  });
});
