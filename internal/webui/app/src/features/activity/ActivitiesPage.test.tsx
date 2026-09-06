/**
 * The activity list, as a reader drives it: the newest ride first, and every
 * row a link to the page that draws it.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activitiesQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, WebUIConfig } from "../../api/types";
import { ActivitiesPage } from "./ActivitiesPage";

const ZONE = "Europe/Berlin";

function config(): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: ZONE,
    identity: { display: "rider@example.test", admin: false },
  };
}

function activity(id: number, startedAt: string): Activity {
  return {
    id,
    startedAt,
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
  };
}

// Given oldest first, so a page that simply prints what it was handed fails.
const ACTIVITIES = [activity(1, "2026-08-19T08:00:00Z"), activity(2, "2026-08-26T08:00:00Z")];

function show(activities: Activity[] | null = ACTIVITIES) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config());
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
  it("lists the rides newest first, each linking to its own page", () => {
    show();

    const links = screen.getAllByRole("link").filter((link) => link.getAttribute("href") !== null);
    const rides = links.filter((link) => link.getAttribute("href")?.startsWith("/activities/"));
    expect(rides.map((link) => link.getAttribute("href"))).toEqual([
      "/activities/2",
      "/activities/1",
    ]);
    expect(rides[0]?.textContent).toContain("30.0 km");
  });

  it("says where a Wahoo account is connected when nothing has been recorded", () => {
    show([]);

    expect(screen.getByRole("link", { name: "settings" })).toHaveAttribute("href", "/settings");
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
});
