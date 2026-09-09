/**
 * The dock's ride history: what each row says about a lap of the route. The
 * stop is only reached for a route with rides, so there is no empty case here —
 * `RouteDock` decides whether the stop exists at all.
 */

import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type { Activity, ActivityRouteMatch } from "../../api/types";
import { riddenOn } from "../../lib/rideHistory";
import { RouteHistory } from "./RouteHistory";

const ROUTE = { provider: "veloplanner", sourceRouteId: 12, stageOrder: 2 };

function match(overrides: Partial<ActivityRouteMatch> = {}): ActivityRouteMatch {
  return { ...ROUTE, routeCoverage: 1, rideCoverage: 1, direction: "forward", ...overrides };
}

function ride(id: number, startedAt: string, overrides: Partial<Activity> = {}): Activity {
  return {
    id: String(id),
    startedAt,
    distanceMetres: 42_000,
    movingSeconds: 5_400,
    elapsedSeconds: 6_000,
    ascentMetres: 600,
    typeId: 40,
    locationId: 0,
    provider: "wahoo",
    routeMatch: match(),
    ...overrides,
  };
}

function show(activities: Activity[]) {
  render(
    <MemoryRouter>
      <RouteHistory rides={riddenOn(activities, ROUTE)} />
    </MemoryRouter>,
  );

  return screen.getByRole("list", { name: "Ride history" });
}

describe("RouteHistory", () => {
  it("lists the rides newest first, each linking to its own page", () => {
    const history = show([ride(1, "2026-08-01T08:00:00Z"), ride(2, "2026-08-10T08:00:00Z")]);

    expect(
      within(history)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/activities/2", "/activities/1"]);
  });

  it("names the best lap and says how much of the route a partial one covered", () => {
    const history = show([
      ride(1, "2026-08-01T08:00:00Z", { movingSeconds: 5_400 }),
      ride(2, "2026-08-10T08:00:00Z", {
        movingSeconds: 3_600,
        routeMatch: match({ routeCoverage: 0.94, direction: "reverse" }),
      }),
    ]);

    expect(within(history).getByText("94% of the route")).toBeInTheDocument();
    expect(within(history).getByText("ridden in reverse")).toBeInTheDocument();
    // The quicker ride covered less of the route, so the whole lap keeps the best.
    expect(within(history).getByText("Best").closest("a")).toHaveAttribute("href", "/activities/1");
  });

  it("writes each ride's own recorded conditions beside it", () => {
    const history = show([
      ride(1, "2026-08-01T08:00:00Z", {
        weather: {
          temperatureMinCelsius: 14.2,
          temperatureMaxCelsius: 23.6,
          windSpeedKmh: 18,
          precipitationMillimetres: 2.5,
          weatherCode: 61,
        },
      }),
    ]);

    expect(within(history).getByText("14–24°, wind 18 km/h, 2.5 mm of rain")).toBeInTheDocument();
    expect(within(history).getByText("28.0 km/h")).toBeInTheDocument();
  });
});
