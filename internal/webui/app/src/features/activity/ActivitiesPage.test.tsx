/**
 * Activities, as a reader drives it.
 *
 * What is asserted is the agreement the page keeps: the totals are the whole
 * window added up, the chart counts weeks and the list months, and a rider
 * with nothing recorded is told where a Wahoo account is connected.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activitiesQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, Status, WebUIConfig } from "../../api/types";
import { IDLE_STATUS } from "../../test/status";
import { ActivitiesPage } from "./ActivitiesPage";

const NOW = new Date(2026, 8, 5, 12); // Saturday 5 September 2026

// A zone well away from the test runner's own, so a test that reads the
// config's zone rather than the browser's fails loudly.
const CONFIG_ZONE = "Pacific/Auckland";

function config(): WebUIConfig {
  return {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: CONFIG_ZONE,
    identity: { display: "rider@example.test", admin: false },
  };
}

function activity(startedAt: Date, overrides: Partial<Activity> = {}): Activity {
  return {
    id: String(startedAt.getTime()),
    startedAt: startedAt.toISOString(),
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
    indoor: false,
    provider: "wahoo",
    ...overrides,
  };
}

// Two rides a week apart, both in August: by week they are a row each with an
// empty week since, by month they are one row of two.
const ACTIVITIES = [activity(new Date(2026, 7, 26, 8)), activity(new Date(2026, 7, 19, 8))];

function show(activities: Activity[] | null = ACTIVITIES, path = "/activities") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config());
  client.setQueryData(statusQuery().queryKey, IDLE_STATUS);
  // The bar's own ⌘K jump reads the library wherever it is mounted.
  client.setQueryData(routesQuery().queryKey, []);
  if (activities) {
    client.setQueryData(activitiesQuery().queryKey, activities);
  }
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ActivitiesPage />
        <Location />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** Where the page has navigated, read back as text. */
function Location() {
  return <span data-testid="location">{useLocation().pathname}</span>;
}

afterEach(() => {
  vi.useRealTimers();
});

describe("the activities page", () => {
  it("adds the window up into the totals", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("60.0 km");
    expect(screen.getByRole("group", { name: "Moving time" })).toHaveTextContent("2 h");
    expect(screen.getByRole("group", { name: "Ascent" })).toHaveTextContent("600 m");
    expect(screen.queryByText(/browser's time zone/)).not.toBeInTheDocument();
  });

  it("includes a ride several years old in the totals once all are asked for", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show([activity(new Date(2021, 7, 26, 8)), ...ACTIVITIES]);
    await userEvent.click(screen.getByRole("button", { name: "All" }));

    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("90.0 km");
    expect(screen.getByRole("group", { name: "Moving time" })).toHaveTextContent("3 h");
    expect(screen.getByRole("group", { name: "Ascent" })).toHaveTextContent("900 m");
  });

  it("charts the weeks and lists the months", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    expect(
      // The year shown runs from its start, weeks nobody rode included.
      screen.getByRole("img", { name: "Distance in each of the last 53 weeks" }),
    ).toBeInTheDocument();
    const months = screen.getByRole("region", { name: "By month" });
    expect(within(months).getByText(/2 rides/)).toBeInTheDocument();
  });

  it("reads a week out without naming a ground nobody rode on", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    const chart = screen.getByRole("img", { name: /^Distance in each/ });
    // From the newest week, which is empty, back to the one holding the ride of 26 August.
    fireEvent.keyDown(chart.parentElement as HTMLElement, { key: "ArrowLeft" });

    const readout = screen.getByRole("status");
    expect(readout).toHaveTextContent("1 ride30.0 km");
    expect(readout).toHaveTextContent("Outdoor");
    expect(readout).not.toHaveTextContent("Indoor");
  });

  it("sets this year against the last and names the records", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show([activity(new Date(2025, 7, 26, 8), { distanceMetres: 90_000 }), ...ACTIVITIES]);
    await userEvent.click(screen.getByRole("button", { name: "All" }));

    const year = screen.getByRole("region", { name: "This year" });
    expect(within(year).getByText("60.0 km")).toBeInTheDocument();
    expect(within(year).getByText("−33%")).toBeInTheDocument();
    const records = screen.getByRole("region", { name: "Records" });
    expect(within(records).getByRole("link", { name: /Longest ride/ })).toHaveAttribute(
      "href",
      `/activities/${new Date(2025, 7, 26, 8).getTime()}`,
    );
  });

  it("counts only the ground the reader picks", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show([
      activity(new Date(2026, 7, 27, 18), { indoor: true, distanceMetres: 25_000 }),
      ...ACTIVITIES,
    ]);

    // Both grounds at once: the whole, then each ground's share beneath it.
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent(
      "85.0 km60.0 km25.0 km",
    );

    await userEvent.click(screen.getByRole("button", { name: "Indoor" }));
    // One ground alone carries no split.
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent(/^Distance25\.0 km$/);

    await userEvent.click(screen.getByRole("button", { name: "Outdoor" }));
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("60.0 km");
  });

  it("says so when no ride was ridden on the ground picked", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    await userEvent.click(screen.getByRole("button", { name: "Indoor" }));
    expect(screen.getByText("No indoor rides in the last year.")).toBeInTheDocument();
  });

  it("counts only the range picked, and keeps this year to its calendar", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show([
      activity(new Date(2025, 11, 1, 8), { distanceMetres: 10_000 }),
      activity(new Date(2026, 2, 1, 8), { distanceMetres: 40_000 }),
      ...ACTIVITIES,
    ]);

    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("110 km");

    await userEvent.click(screen.getByRole("button", { name: "3 months" }));
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("60.0 km");
    expect(
      within(screen.getByRole("region", { name: "This year" })).getByText("100 km"),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "YTD" }));
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("100 km");
  });

  it("sends a rider with nothing recorded to their settings", () => {
    show([]);

    expect(screen.getByText(/No rides have been recorded yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "your account" })).toHaveAttribute(
      "href",
      "/account/accounts",
    );
  });

  it("says so when the service does not answer", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "unavailable" } }), {
            status: 503,
          }),
      ),
    );
    show(null);

    expect(
      await screen.findByText("The service did not say what has been ridden."),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("falls back to the browser zone and says so when the config fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "unavailable" } }), { status: 503 }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    // Seeded so the menu bar's own status query does not also reach the
    // stubbed fetch; only the config query is left to fail.
    const status: Status = {
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
    client.setQueryData(statusQuery().queryKey, status);
    client.setQueryData(activitiesQuery().queryKey, []);
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <ActivitiesPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Periods follow this browser's time zone")).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("keeps one range and ground across the overview and the rides", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show([...ACTIVITIES, activity(new Date(2026, 7, 27, 18), { indoor: true })]);

    await userEvent.click(screen.getByRole("button", { name: "Outdoor" }));
    await userEvent.click(screen.getByRole("button", { name: "Rides" }));

    expect(screen.getByTestId("location")).toHaveTextContent("/activities/rides");
    expect(screen.queryByRole("group", { name: "Distance" })).not.toBeInTheDocument();
    const rides = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("href")?.startsWith("/activities/"));
    expect(rides).toHaveLength(2);

    await userEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/activities$/);
    expect(screen.getByRole("group", { name: "Distance" })).toHaveTextContent("60.0 km");
  });

  it("opens on the rides at their own address", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show(ACTIVITIES, "/activities/rides");

    expect(screen.getByRole("button", { name: "Rides" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getAllByRole("region", { name: /^Week / })).toHaveLength(2);
  });

  it("marks indoor days in the calendar whichever ground is picked", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show(
      [...ACTIVITIES, activity(new Date(2026, 7, 12, 18), { indoor: true })],
      "/activities/rides",
    );

    await userEvent.click(screen.getByRole("button", { name: "Outdoor" }));

    const calendar = screen.getByRole("region", { name: /^Days ridden in August 2026/ });
    expect(within(calendar).getAllByRole("img", { name: /ridden, indoor$/ })).toHaveLength(1);
  });

  it("marks the days ridden in the rides tab's calendar, and not on the overview", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();
    expect(screen.queryByRole("region", { name: /^Days ridden/ })).not.toBeInTheDocument();
    cleanup();
    show(ACTIVITIES, "/activities/rides");

    const calendar = screen.getByRole("region", { name: /^Days ridden in August 2026/ });
    expect(within(calendar).getAllByRole("button", { name: /ridden, outdoor$/ })).toHaveLength(2);
  });

  it("brings a day's ride into view from the calendar, and offers no day the list lacks", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    const scrolled = vi.fn();
    Element.prototype.scrollIntoView = scrolled;
    show(
      [...ACTIVITIES, activity(new Date(2026, 7, 12, 18), { indoor: true })],
      "/activities/rides",
    );

    await userEvent.click(screen.getByRole("button", { name: "Outdoor" }));
    const calendar = screen.getByRole("region", { name: /^Days ridden in August 2026/ });
    expect(within(calendar).queryAllByRole("button", { name: /ridden, indoor$/ })).toHaveLength(0);

    const [day] = within(calendar).getAllByRole("button", { name: /ridden, outdoor$/ });
    await userEvent.click(day as HTMLElement);

    expect(scrolled).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toHaveAttribute("data-highlighted", "true");
    expect(document.activeElement?.getAttribute("href")).toMatch(/^\/activities\/\d+$/);
  });
});
