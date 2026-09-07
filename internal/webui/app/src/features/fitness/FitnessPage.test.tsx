import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Fitness, FitnessDay, Status } from "../../api/types";
import { FitnessPage } from "./FitnessPage";

function day(date: string, overrides: Partial<FitnessDay> = {}): FitnessDay {
  return {
    date,
    trimpLoad: 40,
    trimpFitness: 30,
    trimpFatigue: 45,
    trimpForm: -15,
    tssLoad: 80,
    tssFitness: 60,
    tssFatigue: 90,
    tssForm: -30,
    ...overrides,
  };
}

const STATUS: Status = {
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
    surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
  },
};

const TIMELINE: Fitness = {
  days: [day("2026-08-22"), day("2026-08-23"), day("2026-08-24")],
  weeks: [{ weekStart: "2026-08-17", zoneSeconds: [600, 1200, 300, 0, 0] }],
};

/**
 * The page asks for a window measured from today, so its query key is not one a
 * test can seed. The answer is stubbed at the transport instead.
 */
function show(fitness: Fitness = TIMELINE) {
  // Routed by URL: the page's own chrome polls the status, and answering that
  // with a timeline is what makes the menu bar throw rather than the page fail.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      const body = url.includes("/v1/activities/fitness") ? fitness : STATUS;

      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // PageShell's own chrome reads these; the timeline is what the stub answers.
  client.setQueryData(statusQuery().queryKey, STATUS);
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin: false },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <FitnessPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("FitnessPage", () => {
  it("offers both scales and both are readable", async () => {
    show();

    const group = await screen.findByRole("group", { name: "Scale" });
    expect(group).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "TRIMP" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stress score" })).toBeInTheDocument();
  });

  it("offers the same range control the volume page has", async () => {
    show();

    expect(await screen.findByRole("group", { name: "Range" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "3 months" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1 year" })).toBeInTheDocument();
  });

  it("names the scale the chart is on, and changes it with the toggle", async () => {
    show();

    expect(await screen.findByRole("img", { name: /stress score scale/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "TRIMP" }));
    expect(await screen.findByRole("img", { name: /TRIMP scale/ })).toBeInTheDocument();
  });

  it("sums each week's time in zone beneath the chart", async () => {
    show();

    expect(await screen.findByText("2026-08-17")).toBeInTheDocument();
    expect(screen.getByText("35 min")).toBeInTheDocument();
  });

  // A rider whose rides have not been derived is told what is missing, not shown
  // an empty chart.
  it("says what is missing when nothing has been worked out", async () => {
    show({ days: [], weeks: [] });

    expect(await screen.findByText(/Nothing has been worked out yet/)).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Scale" })).not.toBeInTheDocument();
  });
});
