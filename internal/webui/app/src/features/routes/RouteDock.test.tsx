/**
 * The rail dock: two stops sharing one fixed height, and the pill it folds to.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { weatherQuery } from "../../api/queries";
import {
  climbs,
  coordinates,
  profile,
  rideStart,
  route,
  surface,
  weatherSamples,
} from "../../storybook/fixtures";
import { RouteDock, type RouteDockProps } from "./RouteDock";

function renderDock(overrides: Partial<RouteDockProps> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(weatherQuery(weatherSamples).queryKey, {
    points: weatherSamples.map((sample, index) => ({
      time: sample.arrivalAt.toISOString(),
      temperatureCelsius: 14 + index,
      apparentTemperatureCelsius: 12 + index,
      precipitationMillimetres: 0,
      precipitationProbabilityPercent: 5,
      windSpeedKmh: 10,
      windDirectionDegrees: 180,
      weatherCode: 1,
      cloudCoverPercent: 30,
    })),
  });

  const props: RouteDockProps = {
    title: route.title,
    profile,
    distanceMetres: route.distanceMetres,
    ascentMetres: route.ascentMetres,
    surface,
    climbs,
    onSelectClimb: vi.fn(),
    coordinates,
    samples: weatherSamples,
    startAt: new Date(rideStart.getTime() - 60 * 60_000),
    onStartAtChange: vi.fn(),
    movingSeconds: route.movingSeconds,
    activeMetres: null,
    onActiveChange: vi.fn(),
    zoomWindow: null,
    onZoomChange: vi.fn(),
    highlight: null,
    onHighlightChange: vi.fn(),
    measure: null,
    onMeasureChange: vi.fn(),
    unitSystem: "metric",
    open: true,
    onOpenChange: vi.fn(),
    ...overrides,
  };

  return render(
    <QueryClientProvider client={client}>
      <RouteDock {...props} />
    </QueryClientProvider>,
  );
}

describe("RouteDock", () => {
  it("switches stops by clicking the rail", async () => {
    const user = userEvent.setup();
    renderDock();

    expect(screen.getByRole("img", { name: /^Elevation profile of / })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: /^Forecast along the way/ })).toBeNull();

    await user.click(screen.getByRole("tab", { name: /Forecast/ }));

    expect(screen.queryByRole("img", { name: /^Elevation profile of / })).toBeNull();
    expect(screen.getByRole("tab", { name: /Forecast/ })).toHaveAttribute("aria-selected", "true");
  });

  it("switches stops by arrow key then activation", async () => {
    const user = userEvent.setup();
    renderDock();

    const profileTab = screen.getByRole("tab", { name: /Profile/ });
    const forecastTab = screen.getByRole("tab", { name: /Forecast/ });
    profileTab.focus();
    await user.keyboard("{ArrowDown}");
    expect(forecastTab).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(forecastTab).toHaveAttribute("aria-selected", "true");
  });

  it("gives both stops the same fixed-height wrapper", async () => {
    const user = userEvent.setup();
    const { container } = renderDock();

    const wrapper = container.querySelector(".h-52");
    expect(wrapper).not.toBeNull();

    await user.click(screen.getByRole("tab", { name: /Forecast/ }));

    expect(container.querySelector(".h-52")).toBe(wrapper);
  });

  it("shows the hover readout on the profile line while a position is active", () => {
    renderDock({ activeMetres: 500 });

    expect(screen.getByLabelText("Elevation summary")).toHaveTextContent(/%/);
  });

  it("lists the steepness bands behind the profile info control", async () => {
    const user = userEvent.setup();
    renderDock();

    await user.click(screen.getByRole("button", { name: "More about this" }));

    expect(await screen.findByText("Steepness")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Up" })).toBeInTheDocument();
  });

  it("shows the forecast resolution sentence behind its info control", async () => {
    const user = userEvent.setup();
    renderDock();
    await user.click(screen.getByRole("tab", { name: /Forecast/ }));

    await user.click(screen.getByRole("button", { name: "More about this" }));

    expect(await screen.findByText(/resolution/)).toBeInTheDocument();
  });

  it("folds to a strip and back on the last-shown stop", async () => {
    const user = userEvent.setup();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(weatherQuery(weatherSamples).queryKey, { points: [] });

    function Controlled() {
      const [open, setOpen] = useState(true);
      return (
        <RouteDock
          title={route.title}
          profile={profile}
          distanceMetres={route.distanceMetres}
          ascentMetres={route.ascentMetres}
          surface={surface}
          climbs={climbs}
          onSelectClimb={vi.fn()}
          coordinates={coordinates}
          samples={weatherSamples}
          startAt={null}
          onStartAtChange={vi.fn()}
          activeMetres={null}
          onActiveChange={vi.fn()}
          zoomWindow={null}
          onZoomChange={vi.fn()}
          highlight={null}
          onHighlightChange={vi.fn()}
          measure={null}
          onMeasureChange={vi.fn()}
          unitSystem="metric"
          open={open}
          onOpenChange={setOpen}
        />
      );
    }
    render(
      <QueryClientProvider client={client}>
        <Controlled />
      </QueryClientProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Hide the route detail" }));
    expect(screen.getByRole("group", { name: "Route detail, folded" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show the profile" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show the forecast" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show the forecast" }));
    expect(screen.getByRole("tab", { name: /Forecast/ })).toHaveAttribute("aria-selected", "true");

    await user.click(screen.getByRole("button", { name: "Hide the route detail" }));
    await user.click(screen.getByRole("button", { name: "Show the route detail" }));
    expect(screen.getByRole("tab", { name: /Forecast/ })).toHaveAttribute("aria-selected", "true");
  });
});
