/**
 * Volume, as a reader drives it.
 *
 * What is asserted is the agreement the page keeps: the totals are the whole
 * window added up, the toggle changes which period the rows count, and a rider
 * with nothing recorded is told where a Wahoo account is connected.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activitiesQuery } from "../../api/queries";
import type { Activity } from "../../api/types";
import { windowStart } from "../../lib/volume";
import { VolumePage } from "./VolumePage";

const NOW = new Date(2026, 8, 5, 12); // Saturday 5 September 2026

function activity(startedAt: Date, overrides: Partial<Activity> = {}): Activity {
  return {
    id: startedAt.getTime(),
    startedAt: startedAt.toISOString(),
    distanceMetres: 30_000,
    movingSeconds: 3_600,
    elapsedSeconds: 4_000,
    ascentMetres: 300,
    typeId: 40,
    locationId: 0,
    ...overrides,
  };
}

// Two rides a week apart, both in August: by week they are a row each with an
// empty week since, by month they are one row of two.
const ACTIVITIES = [activity(new Date(2026, 7, 26, 8)), activity(new Date(2026, 7, 19, 8))];

function show(activities: Activity[] | null = ACTIVITIES) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (activities) {
    client.setQueryData(activitiesQuery(windowStart()).queryKey, activities);
  }
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <VolumePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.useRealTimers();
});

describe("the volume page", () => {
  it("adds the window up into the totals", () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    expect(screen.getByText("60.0 km")).toBeInTheDocument();
    expect(screen.getByText("2 h")).toBeInTheDocument();
    expect(screen.getByText("600 m")).toBeInTheDocument();
  });

  it("counts by month once the reader asks for months", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(NOW);
    show();

    expect(screen.getByRole("heading", { name: "By week", level: 2 })).toBeInTheDocument();
    expect(screen.getAllByText(/1 ride/)).toHaveLength(2);

    await userEvent.click(screen.getByRole("button", { name: "Month" }));

    expect(screen.getByRole("heading", { name: "By month", level: 2 })).toBeInTheDocument();
    expect(screen.getByText(/2 rides/)).toBeInTheDocument();
  });

  it("sends a rider with nothing recorded to their settings", () => {
    show([]);

    expect(screen.getByText(/No rides have been recorded yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "settings" })).toHaveAttribute("href", "/settings");
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
});
