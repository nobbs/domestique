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

const withDocument: Activity = {
  ...ride,
  metrics: { trimp: 42 },
  analysis: {
    text: "A steady endurance ride.",
    model: "claude-sonnet-5",
    promptRevision: 3,
    analysedAt: "2026-09-01T08:00:00Z",
    document: {
      rideType: "endurance",
      headline: "Solid endurance work",
      summary: "Steady effort throughout, in zone the whole way.",
      loadEffect: "Adds a moderate training load.",
      highlights: ["Held power steady on the climb"],
      concerns: ["Cadence dropped in the last hour"],
      nextSession: { advice: "Recover easy tomorrow.", suggestedRestDays: 1 },
      dataGaps: ["power meter offline for the first 10 minutes"],
    },
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

  it("renders each part of a structured document", () => {
    show(withDocument);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("Solid endurance work");
    expect(section).toHaveTextContent("endurance");
    expect(section).toHaveTextContent("Steady effort throughout, in zone the whole way.");
    expect(section).toHaveTextContent("Adds a moderate training load.");
    expect(section).toHaveTextContent("Held power steady on the climb");
    expect(section).toHaveTextContent("Cadence dropped in the last hour");
    expect(section).toHaveTextContent("Recover easy tomorrow.");
    expect(section).toHaveTextContent("Suggested rest: 1 day(s).");
    expect(section).toHaveTextContent("power meter offline for the first 10 minutes");
    expect(section).toHaveTextContent("claude-sonnet-5");
  });

  it("falls back to the text when a document is absent", () => {
    show(analysed);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("A steady endurance ride.");
    expect(section).not.toHaveTextContent("Suggested rest");
  });

  it("omits empty lists and a zero rest suggestion", () => {
    const noRest: Activity = {
      ...ride,
      metrics: { trimp: 42 },
      analysis: {
        text: "A steady endurance ride.",
        model: "claude-sonnet-5",
        promptRevision: 3,
        analysedAt: "2026-09-01T08:00:00Z",
        document: {
          rideType: "endurance",
          headline: "Solid endurance work",
          summary: "Steady effort throughout.",
          loadEffect: "Adds a moderate training load.",
          highlights: [],
          concerns: [],
          nextSession: { advice: "Keep it easy.", suggestedRestDays: 0 },
          dataGaps: [],
        },
      },
    };
    show(noRest);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).not.toHaveTextContent("Highlights");
    expect(section).not.toHaveTextContent("Concerns");
    expect(section).not.toHaveTextContent("Missing figures");
    expect(section).not.toHaveTextContent("Suggested rest");
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
