import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activityHeartRateDistributionQuery } from "../../api/queries";
import type { ActivityHeartRateDistribution } from "../../api/types";
import { bucketBeats, HeartRateZones, type HeartRateZonesProps } from "./HeartRateZones";

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

  it("draws the heart rates the ride held, coloured by the zone they fall in", async () => {
    const { container } = show({}, { fromBpm: 118, seconds: [5, 10, 0, 20] });

    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    // 119.5 is cut at 120, which cuts the 115-119 and 120-124 bars at the ends held.
    expect(bars(container).map((rect) => rect.getAttribute("fill"))).toEqual([
      "var(--grade-0)",
      "var(--grade-1)",
    ]);
    expect(screen.getByText("120")).toBeInTheDocument();
    // The table stays, so the zones are still read beside the spread.
    expect(screen.getAllByRole("row")).toHaveLength(5);
  });

  it("lights the pointed bar's zone in the table and reads the bar out", async () => {
    const { container } = show({}, { fromBpm: 118, seconds: [5, 10, 0, 25] });
    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    fireEvent.mouseEnter(container.querySelectorAll("rect:not([data-bar])")[1] as Element);

    expect(screen.getAllByRole("row")[1]).toHaveAttribute("data-active");
    expect(screen.getByText("120–121 bpm")).toBeInTheDocument();
    expect(screen.getByText("25 s · 63%")).toBeInTheDocument();
  });

  it("draws a wide spread in five-beat bars, cut at each zone edge", async () => {
    const { container } = show({}, { fromBpm: 100, seconds: Array.from({ length: 100 }, () => 6) });
    await userEvent.click(screen.getByRole("button", { name: "Distribution" }));

    // Twenty five-beat bars, two of them cut in two where 134 and 148 fall inside.
    expect(bars(container)).toHaveLength(22);
    fireEvent.mouseEnter(container.querySelectorAll("rect:not([data-bar])")[7] as Element);
    expect(screen.getByText("134 bpm")).toBeInTheDocument();
    expect(screen.getAllByRole("row")[2]).toHaveAttribute("data-active");
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

describe("bucketBeats", () => {
  it("groups five beats to a bar on round numbers, however narrow the spread", () => {
    const buckets = bucketBeats({ fromBpm: 130, seconds: Array.from({ length: 10 }, () => 1) }, []);

    expect(buckets.map((bucket) => [bucket.fromBpm, bucket.toBpm, bucket.seconds])).toEqual([
      [130, 134, 5],
      [135, 139, 5],
    ]);
  });

  it("cuts a bar at the zone edge inside it", () => {
    const buckets = bucketBeats(
      { fromBpm: 120, seconds: Array.from({ length: 10 }, () => 1) },
      [123],
    );

    expect(buckets).toEqual([
      { fromBpm: 120, toBpm: 122, seconds: 3, zone: 0 },
      { fromBpm: 123, toBpm: 124, seconds: 2, zone: 1 },
      { fromBpm: 125, toBpm: 129, seconds: 5, zone: 1 },
    ]);
  });

  it("cuts the first and last bars where the distribution starts and ends", () => {
    const buckets = bucketBeats({ fromBpm: 103, seconds: Array.from({ length: 9 }, () => 1) }, []);

    expect(buckets).toEqual([
      { fromBpm: 103, toBpm: 104, seconds: 2, zone: 0 },
      { fromBpm: 105, toBpm: 109, seconds: 5, zone: 0 },
      { fromBpm: 110, toBpm: 111, seconds: 2, zone: 0 },
    ]);
  });
});
