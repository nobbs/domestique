import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, taskRunsQueryKey, tasksQuery } from "../../../api/queries";
import type { Status, Task, TaskList } from "../../../api/types";
import { TaskTable } from "./TaskTable";

function taskList(patch: Partial<Record<string, Partial<Task>>> = {}): TaskList {
  const defaults: Task[] = [
    {
      name: "sync:source",
      scheduled: true,
      enabled: true,
      running: 0,
      intervalSeconds: 3600,
      nextRunAt: "2026-08-31T08:00:00Z",
    },
    { name: "sync:target", scheduled: true, enabled: true, running: 0, intervalSeconds: 21600 },
    { name: "sync:clear", scheduled: false, enabled: true, running: 0 },
    { name: "surface:annotate", scheduled: false, enabled: true, running: 0 },
  ];

  return { tasks: defaults.map((task) => ({ ...task, ...(patch[task.name] ?? {}) })) };
}

function status(targets: string[] = ["rider-a", "rider-b"]): Status {
  return {
    ready: true,
    converged: true,
    targets: targets.map((id) => ({
      id,
      authorisation: "authorized",
      convergence: "current",
      routes: { current: 0, pending: 0 },
    })),
    sync: {
      state: "idle",
      sourceRoutes: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
    },
  };
}

function renderTable(tasks: TaskList = taskList(), statusValue: Status = status()) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(tasksQuery().queryKey, tasks);
  client.setQueryData(statusQuery().queryKey, statusValue);

  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <TaskTable />
      </QueryClientProvider>,
    ),
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TaskTable", () => {
  it("shows each task's name, cadence, and when it is next due", () => {
    renderTable();

    expect(screen.getByText("sync:source")).toBeInTheDocument();
    expect(screen.getByText("Hourly")).toBeInTheDocument();
    expect(screen.getByText("Every 6 hours")).toBeInTheDocument();
    // sync:clear and surface:annotate are nothing this build schedules.
    expect(screen.getAllByText("On demand")).toHaveLength(2);
    // Three report no next run, and the two nothing schedules are offered no
    // switch either, because nothing would read it.
    expect(screen.getAllByText("—")).toHaveLength(5);
    expect(screen.getAllByRole("switch")).toHaveLength(2);
  });

  it("shows a badge while a task has an attempt in flight", () => {
    renderTable(taskList({ "sync:source": { running: 1 } }));

    expect(screen.getByText("Running")).toBeInTheDocument();
  });

  it("calls the schedule mutation from a task's own switch", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") {
        return new Response(JSON.stringify(taskList({ "sync:source": { enabled: false } })), {
          status: 200,
        });
      }

      return new Response(JSON.stringify(taskList()));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderTable();

    await userEvent.click(screen.getByRole("switch", { name: "Scheduled: sync:source" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((call) => call[0] === "/v1/tasks/sync%3Asource/schedule"),
      ).toBe(true),
    );
    const call = fetchMock.mock.calls.find((c) => c[0] === "/v1/tasks/sync%3Asource/schedule");
    expect(call?.[1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ enabled: false }),
    });
  });

  // Turning a schedule off is a statement about unattended runs, not a lock:
  // the operator asking for a run has already decided.
  it("keeps the run button enabled for a task whose schedule is switched off", () => {
    renderTable(taskList({ "sync:source": { enabled: false } }));

    expect(screen.getByRole("button", { name: "Run now: sync:source" })).toBeEnabled();
  });

  it("offers no run button for the task only the Wahoo receiver starts", () => {
    renderTable({
      tasks: [{ name: "activity:record", scheduled: false, enabled: true, running: 0 }],
    });

    expect(screen.getByText("activity:record")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Run now: activity:record" })).toBeNull();
  });

  it("posts a run for the pressed task", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ status: "accepted" }), { status: 202 });
      }

      return new Response(JSON.stringify(taskList()));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Run now: sync:target" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith("/v1/tasks/sync%3Atarget/run", expect.anything()),
    );
  });

  // A 409 is not a fault: it means this exact work is already happening, or
  // something it needs is held by another run.
  it("reports a rejected run as busy rather than as a failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: {
                code: "task_in_progress",
                message:
                  "the task is already running, or something it needs is held by another run",
              },
            }),
            { status: 409 },
          ),
      ),
    );
    renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Run now: sync:source" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "the task is already running, or something it needs is held by another run",
    );
  });

  // One mutation serves every row, so an ungated spinner would report each
  // other task as running too.
  it("shows progress only in the row whose run was asked for", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          await new Promise<Response>(() => {
            // Never settles: the button stays mid-flight for the assertion.
          }),
      ),
    );
    renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Run now: sync:source" }));

    expect(await screen.findByLabelText("Running sync:source")).toBeInTheDocument();
    expect(screen.queryByLabelText("Running sync:target")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run now: sync:target" })).toBeEnabled();
  });

  // The refusal is recorded as a skipped attempt, and the history beneath this
  // table is where an operator reads why a run did not happen.
  it("refreshes the history after a run is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "task_in_progress", message: "busy" } }), {
            status: 409,
          }),
      ),
    );
    const { client } = renderTable();
    client.setQueryData(taskRunsQueryKey(), { pages: [], pageParams: [] });

    await userEvent.click(screen.getByRole("button", { name: "Run now: sync:source" }));

    await waitFor(() => expect(client.getQueryState(taskRunsQueryKey())?.isInvalidated).toBe(true));
  });

  // sync:clear deletes every owned route from one slot, and a run with none
  // named does nothing: nothing may be posted until one is chosen and typed.
  it("will not run sync:clear until a slot is chosen and confirmed", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ status: "accepted" }), { status: 202 });
      }

      return new Response(JSON.stringify(taskList()));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Run now: sync:clear" }));

    const confirm = screen.getByRole("button", { name: "Delete them" });
    expect(confirm).toBeDisabled();

    await userEvent.click(screen.getByRole("radio", { name: "rider-a" }));
    expect(confirm).toBeDisabled();

    // The wrong name is not enough, even once a slot is chosen.
    await userEvent.type(screen.getByLabelText(/Type/), "rider-b");
    expect(confirm).toBeDisabled();

    await userEvent.clear(screen.getByLabelText(/Type/));
    await userEvent.type(screen.getByLabelText(/Type/), "rider-a");
    expect(confirm).toBeEnabled();

    await userEvent.click(confirm);

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((call) => call[0] === "/v1/tasks/sync%3Aclear/run/rider-a"),
      ).toBe(true),
    );
  });
});
