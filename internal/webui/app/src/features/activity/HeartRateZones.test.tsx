import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activityHeartRateDistributionQuery } from "../../api/queries";
import type { ActivityHeartRateDistribution } from "../../api/types";
import { HeartRateZones, type HeartRateZonesProps } from "./HeartRateZones";

const ZONES: HeartRateZonesProps = {
  rideId: "7",
  zoneSeconds: [60, 120, 180, 240, 300],
  zoneBounds: [119.5, 134, 140, 147.2],
  deviceZoneSeconds: undefined,
  coverage: undefined,
};

function show(
  props: Partial<HeartRateZonesProps> = {},
  distribution?: ActivityHeartRateDistribution,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (distribution) {
    client.setQueryData(activityHeartRateDistributionQuery("7").queryKey, distribution);
  }

  return render(
    <QueryClientProvider client={client}>
      <HeartRateZones {...ZONES} {...props} />
    </QueryClientProvider>,
  );
}

function bars(container: HTMLElement): SVGRectElement[] {
  return Array.from(container.querySelectorAll<SVGRectElement>("rect[data-bar]"));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("HeartRateZones", () => {
  // The default view asks the service for nothing; the suite's fetch guard
  // fails any test that renders one that does.
  it("opens on the ring, with the whole time at its centre", () => {
    show();

    expect(screen.getByRole("button", { name: "Zones" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("15 min")).toBeInTheDocument();
    expect(screen.getByText("in zones")).toBeInTheDocument();
  });

  it("names the pointed zone at the ring's centre", () => {
    show();

    fireEvent.mouseEnter(screen.getAllByRole("row")[2] as Element);

    expect(screen.getByText("Tempo", { selector: "span" })).toBeInTheDocument();
    expect(screen.getAllByText("3 min")).toHaveLength(2);
  });

  it("offers no distribution for a row derived without its bounds", () => {
    show({ zoneBounds: undefined });

    expect(screen.queryByRole("button", { name: "Distribution" })).not.toBeInTheDocument();
  });

  it("draws every whole heart rate the ride held, coloured by the zone it falls in", async () => {
    const { container } = show({}, { fromBpm: 118, seconds: [5, 10, 0, 20] });

    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    const drawn = bars(container);
    expect(drawn).toHaveLength(4);
    // 119.5 is cut at 120: 118 and 119 are Recovery, 120 and 121 Endurance.
    expect(drawn.map((rect) => rect.getAttribute("fill"))).toEqual([
      "var(--grade-0)",
      "var(--grade-0)",
      "var(--grade-1)",
      "var(--grade-1)",
    ]);
    expect(screen.getByText("120")).toBeInTheDocument();
    // The table stays, so the zones are still read beside the spread.
    expect(screen.getAllByRole("row")).toHaveLength(5);
  });

  it("lights the pointed bar's zone in the table and reads the bar out", async () => {
    const { container } = show({}, { fromBpm: 118, seconds: [5, 10, 0, 25] });
    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    fireEvent.mouseEnter(container.querySelectorAll("rect:not([data-bar])")[3] as Element);

    expect(screen.getAllByRole("row")[1]).toHaveAttribute("data-active");
    expect(screen.getByText("121 bpm")).toBeInTheDocument();
    expect(screen.getByText("25 s · 63%")).toBeInTheDocument();
  });

  it("says so when the service holds no heart rates for the ride", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 404 })),
    );
    show();

    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    await waitFor(() =>
      expect(screen.getByText("No heart rates are stored for this ride.")).toBeInTheDocument(),
    );
  });

  it("says so when the service did not answer", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 503 })),
    );
    show();

    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    await waitFor(() =>
      expect(
        screen.getByText("The service did not say how long the ride held each heart rate."),
      ).toBeInTheDocument(),
    );
  });
});
