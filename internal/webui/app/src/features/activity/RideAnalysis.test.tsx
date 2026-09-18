import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { tasksQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, TaskList, WebUIConfig } from "../../api/types";
import { RideAnalysis } from "./RideAnalysis";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ride: Activity = {
  id: "1",
  startedAt: "2026-09-01T06:00:00Z",
  distanceMetres: 36000,
  movingSeconds: 3600,
  elapsedSeconds: 4000,
  ascentMetres: 420,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
};

const analysed: Activity = {
  ...ride,
  metrics: { trimp: 42 },
  analysis: {
    text: "A steady endurance ride.\n\nKeep tomorrow easy.",
    model: "claude-sonnet-5",
    promptRevision: 1,
    analysedAt: "2026-09-01T08:00:00Z",
  },
};

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

const withReanalyse: TaskList = {
  tasks: [{ name: "activity:reanalyse", scheduled: false, enabled: true, running: 0 }],
};

function show(value: Activity, admin = false, tasks: TaskList = withReanalyse) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));
  client.setQueryData(tasksQuery().queryKey, tasks);

  return render(
    <QueryClientProvider client={client}>
      <RideAnalysis ride={value} />
    </QueryClientProvider>,
  );
}

describe("RideAnalysis", () => {
  it("shows the text as written, with the model that wrote it", () => {
    show(analysed);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("A steady endurance ride.");
    expect(section).toHaveTextContent("Keep tomorrow easy.");
    expect(section).toHaveTextContent("claude-sonnet-5");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows nothing for a ride not analysed", () => {
    const { container } = show(ride);

    expect(container).toBeEmptyDOMElement();
  });

  it("lets an admin ask again about an analysed ride", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" }), { status: 202 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(analysed, true);

    await userEvent.click(screen.getByRole("button", { name: "Analyse again" }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.some((call) => call[0] === "/v1/activities/1/reanalyse")).toBe(
        true,
      ),
    );
    expect(await screen.findByText(/Reload the page in a minute/)).toBeInTheDocument();
  });

  it("says so when the request is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response(JSON.stringify({ error: "task_in_progress", message: "busy" }), {
            status: 409,
          }),
      ),
    );
    show(analysed, true);

    await userEvent.click(screen.getByRole("button", { name: "Analyse again" }));

    expect(await screen.findByText(/did not take the request/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Analyse again" })).toBeEnabled();
  });

  it("offers an admin a first analysis of a derived ride", () => {
    show({ ...ride, metrics: { trimp: 42 } }, true);

    expect(screen.getByRole("button", { name: "Analyse" })).toBeInTheDocument();
  });

  it("offers nothing where analysis is off or the ride is not derived", () => {
    const { container } = show({ ...ride, metrics: { trimp: 42 } }, true, { tasks: [] });
    expect(container).toBeEmptyDOMElement();

    const underived = show(ride, true);
    expect(underived.container).toBeEmptyDOMElement();
  });
});
