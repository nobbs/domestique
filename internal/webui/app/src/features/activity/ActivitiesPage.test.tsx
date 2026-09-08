/**
 * The activity list, as a reader drives it: weeks newest first, every ride a
 * link to the page that draws it, its weekday and week decided in the
 * service's own time zone.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activitiesQuery, routesQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, ActivityRouteMatch, Route, WebUIConfig } from "../../api/types";
import { formatAscent, formatDistance, formatDuration, formatTimestamp } from "../../lib/format";
import { ActivitiesPage } from "./ActivitiesPage";

/** The day a week's label starts with, in the platform's own locale. */
function weekOf(day: number): string {
  return new Intl.DateTimeFormat(undefined, {
    day: "numeric",
    month: "short",
    timeZone: ZONE,
  }).format(new Date(Date.UTC(2026, 7, day, 12)));
}

const ZONE = "Europe/Berlin";

function config(): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: ZONE,
    identity: { display: "rider@example.test", admin: false },
  };
}

function activity(id: number, startedAt: string, overrides: Partial<Activity> = {}): Activity {
  return {
    id,
    startedAt,
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
    ...overrides,
  };
}

// Given oldest first, so a page that simply prints what it was handed fails.
const ACTIVITIES = [activity(1, "2026-08-19T08:00:00Z"), activity(2, "2026-08-26T08:00:00Z")];

const LIBRARY_ROUTE: Route = {
  provider: "veloplanner",
  sourceRouteId: 12,
  stageOrder: 2,
  title: "Alpine loop — Descent",
  sourceRouteName: "Alpine loop",
  routeName: "Descent",
  sourceRevision: "2026-08-17",
  contentHash: "hash",
  distanceMetres: 30_000,
  ascentMetres: 300,
  descentMetres: 300,
  maxGradientPercent: 8,
  pointCount: 900,
};

/** A match onto the library route above, as a recorded ride carries one. */
function matchedTo(route: Route): ActivityRouteMatch {
  return {
    provider: route.provider,
    sourceRouteId: route.sourceRouteId,
    stageOrder: route.stageOrder,
    routeCoverage: 1,
    rideCoverage: 0.98,
    direction: "forward",
  };
}

function show(activities: Activity[] | null = ACTIVITIES, library: Route[] | null = []) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config());
  // Null leaves it unseeded, which is the only way to see whether the page asks.
  if (library) {
    client.setQueryData(routesQuery().queryKey, library);
  }
  if (activities) {
    client.setQueryData(activitiesQuery().queryKey, activities);
  }
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ActivitiesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the activity list", () => {
  it("lists weeks newest first, each ride linking to its own page", () => {
    show();

    const names = screen.getAllByRole("region").map((region) => region.getAttribute("aria-label"));
    const newer = names.findIndex((name) => name?.startsWith(`Week ${weekOf(24)}`));
    const older = names.findIndex((name) => name?.startsWith(`Week ${weekOf(17)}`));
    expect(newer).toBeGreaterThanOrEqual(0);
    expect(older).toBeGreaterThan(newer);

    const links = screen.getAllByRole("link").filter((link) => link.getAttribute("href") !== null);
    const rides = links.filter((link) => link.getAttribute("href")?.startsWith("/activities/"));
    expect(rides.map((link) => link.getAttribute("href"))).toEqual([
      "/activities/2",
      "/activities/1",
    ]);
    expect(rides[0]?.textContent).toContain("30.0 km");
  });

  // Two rides of the same distance would otherwise be two links of the same name.
  it("names each ride link by when it started", () => {
    show();

    expect(
      screen.getByRole("link", { name: new RegExp(formatTimestamp(ACTIVITIES[1]?.startedAt)) }),
    ).toHaveAttribute("href", "/activities/2");
  });

  it("says where a Wahoo account is connected when nothing has been recorded", () => {
    show([]);

    expect(screen.getByRole("link", { name: "settings" })).toHaveAttribute("href", "/settings");
  });

  it("shows a ride's max temperature and rain when the service reported them", () => {
    show([
      {
        ...(ACTIVITIES[0] as Activity),
        weather: {
          temperatureMinCelsius: 11.6,
          temperatureMaxCelsius: 18.2,
          windSpeedKmh: 14,
          precipitationMillimetres: 2.4,
          weatherCode: 61,
        },
      },
    ]);

    expect(screen.getByText("18°")).toBeInTheDocument();
    expect(screen.getByText("2.4 mm")).toBeInTheDocument();
  });

  // A dry ride says nothing about rain rather than saying none fell.
  it("leaves rain out of a dry ride", () => {
    show([
      {
        ...(ACTIVITIES[0] as Activity),
        weather: {
          temperatureMinCelsius: 15,
          temperatureMaxCelsius: 15,
          windSpeedKmh: 9,
          precipitationMillimetres: 0,
          weatherCode: 0,
        },
      },
    ]);

    expect(screen.getByText("15°")).toBeInTheDocument();
    expect(screen.queryByText(/mm/)).not.toBeInTheDocument();
  });

  it("says nothing about weather for a ride nobody has asked the weather about", () => {
    show([ACTIVITIES[0] as Activity]);

    expect(screen.queryByText(/°/)).not.toBeInTheDocument();
  });

  it("places a ride in the weekday column of the service's own zone, not UTC", () => {
    // 2026-08-23T22:30:00Z is 00:30 Monday 24 Aug in Europe/Berlin, still
    // Sunday 23 Aug in UTC — the week and the weekday both hinge on the zone.
    show([activity(3, "2026-08-23T22:30:00Z")]);

    const week = screen.getByRole("region", { name: new RegExp(`^Week ${weekOf(24)}`) });
    const monday = within(week).getByRole("group", { name: "Mon" });
    expect(within(monday).getByRole("link")).toHaveAttribute("href", "/activities/3");
  });

  it("scales a bar's height against the longest ride on the page", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", { distanceMetres: 60_000 }),
      activity(2, "2026-08-19T09:00:00Z", { distanceMetres: 30_000 }),
    ]);

    const longBar = document.querySelector<HTMLElement>(
      'a[href="/activities/1"] [aria-hidden="true"]',
    );
    const shortBar = document.querySelector<HTMLElement>(
      'a[href="/activities/2"] [aria-hidden="true"]',
    );
    const longHeight = Number.parseFloat(longBar?.style.height ?? "");
    const shortHeight = Number.parseFloat(shortBar?.style.height ?? "");
    expect(shortHeight).toBeCloseTo(longHeight / 2);
  });

  it("adds a week's rides into its totals line", () => {
    show([
      activity(1, "2026-08-19T08:00:00Z", {
        distanceMetres: 40_000,
        movingSeconds: 3_600,
        ascentMetres: 300,
      }),
      activity(2, "2026-08-20T08:00:00Z", {
        distanceMetres: 20_000,
        movingSeconds: 1_800,
        ascentMetres: 150,
      }),
    ]);

    const week = screen.getByRole("region", { name: new RegExp(`^Week ${weekOf(17)}`) });
    expect(within(week).getByText(formatDistance(60_000))).toBeInTheDocument();
    expect(
      within(week).getByText(`${formatDuration(5_400)} · ${formatAscent(450)}`),
    ).toBeInTheDocument();
  });

  it("waits for the activities rather than claiming there are none", () => {
    // Uncached, so React Query falls through to a real fetch; stub it so the
    // request never settles and the page stays in its loading state.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => {})),
    );
    show(null);

    expect(screen.getByRole("status", { name: "Loading activities" })).toBeInTheDocument();
  });
  it("offers no route filter, and asks for no library, while no ride is matched", () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL) => new Response(null, { status: 500 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(ACTIVITIES, null);

    expect(screen.queryByRole("combobox", { name: "Route" })).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/v1/routes"))).toBe(false);
  });

  it("narrows the weeks to the rides of one route, and back again", async () => {
    show(
      [
        activity(1, "2026-08-19T08:00:00Z"),
        activity(2, "2026-08-26T08:00:00Z", { routeMatch: matchedTo(LIBRARY_ROUTE) }),
      ],
      [LIBRARY_ROUTE],
    );

    const filter = screen.getByRole("combobox", { name: "Route" });
    await userEvent.selectOptions(filter, "veloplanner/12/2");
    const shown = screen
      .getAllByRole("link")
      .map((link) => link.getAttribute("href"))
      .filter((href) => href?.startsWith("/activities/"));
    expect(shown).toEqual(["/activities/2"]);

    await userEvent.selectOptions(filter, "");
    expect(
      screen
        .getAllByRole("link")
        .map((link) => link.getAttribute("href"))
        .filter((href) => href?.startsWith("/activities/")),
    ).toHaveLength(2);
  });
});
