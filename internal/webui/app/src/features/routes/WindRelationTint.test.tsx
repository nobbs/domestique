/**
 * The tint on the route, asked about without a canvas.
 *
 * The same approach `ConditionsWash.test.tsx` takes: MapLibre's bindings are
 * fakes that record what they were handed, so a question about the ink can be
 * asked of a map that never draws anything. The stretches themselves are handed
 * in rather than derived here — `forecastCells.test.ts` is where the reading of
 * the road is examined; this is about what is drawn from it, and what a reader
 * who is not looking at the map has instead.
 */

import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Position } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { windRelationColour, windRelationWords } from "../../lib/measures";
import type { CoordinateRange } from "../../lib/profile";
import type { WindRun } from "./forecastCells";

interface LayerRecord {
  id: string;
  type: string;
  beforeId?: string;
  paint: Record<string, unknown>;
}

interface SourceRecord {
  id: string;
  data: unknown;
}

/** One stretch as the layer receives it: a line, a colour and whether it is lit. */
interface TintFeature {
  geometry: { type: string; coordinates: Position[][] };
  properties: { colour: string; shown: boolean };
}

const drawn = vi.hoisted(() => ({
  layers: [] as LayerRecord[],
  sources: [] as SourceRecord[],
}));

vi.mock("react-map-gl/maplibre", () => ({
  Layer: (props: LayerRecord) => {
    drawn.layers.push(props);

    return null;
  },
  Source: (props: SourceRecord & { children?: ReactNode }) => {
    drawn.sources.push({ id: props.id, data: props.data });

    return <>{props.children}</>;
  },
}));

const { WIND_TINT_LAYER_ID, WindRelationTint } = await import("./WindRelationTint");

/** A due-east road of 3 km, drawn a point every 150 m. */
const ROAD: Position[] = Array.from(
  { length: 21 },
  (_, index): Position => [8 + index * 0.00206, 49],
);

const HEADWIND: WindRun = { fromMetres: 0, toMetres: 1_500, stop: 0, windSpeedKmh: 24 };
const SHIFTING: WindRun = { fromMetres: 1_500, toMetres: 3_000, stop: null, windSpeedKmh: 24 };

function show(options: {
  runs?: WindRun[];
  lit?: readonly CoordinateRange[] | null;
  dark?: boolean;
}) {
  return render(
    <CartographyProvider dark={options.dark ?? false}>
      <WindRelationTint
        runs={options.runs ?? [HEADWIND, SHIFTING]}
        coordinates={ROAD}
        lit={options.lit ?? null}
        beforeId="route-casing"
      />
    </CartographyProvider>,
  );
}

function tintLayer(): LayerRecord | undefined {
  return drawn.layers.find((layer) => layer.id === WIND_TINT_LAYER_ID);
}

function tintFeatures(): TintFeature[] {
  const data = drawn.sources[0]?.data as { features?: TintFeature[] } | undefined;

  return data?.features ?? [];
}

beforeEach(() => {
  drawn.layers = [];
  drawn.sources = [];
});

describe("the route drawn in what the wind is doing", () => {
  it("draws each stretch in its own stop of the ramp", () => {
    show({});

    expect(tintFeatures().map((feature) => feature.properties.colour)).toEqual([
      windRelationColour(0, false),
      windRelationColour(null, false),
    ]);
  });

  it("draws a shifting stretch in the neutral, not in the middle of the ramp", () => {
    show({ runs: [SHIFTING] });
    const [drawnColour] = tintFeatures().map((feature) => feature.properties.colour);

    expect(drawnColour).toBe(windRelationColour(null, false));
    for (const stop of [0, 1, 2, 3]) {
      expect(drawnColour).not.toBe(windRelationColour(stop, false));
    }
  });

  it("takes the ramp of the cartography actually loaded", () => {
    show({ dark: true });

    expect(tintFeatures()[0]?.properties.colour).toBe(windRelationColour(0, true));
  });

  it("sits under the casing, where the steepness edging would be", () => {
    show({});

    expect(tintLayer()?.type).toBe("line");
    expect(tintLayer()?.beforeId).toBe("route-casing");
  });

  it("cuts each stretch out of the route, sharing a point and no segment", () => {
    show({});
    const [head, shifting] = tintFeatures();
    const headLine = head?.geometry.coordinates[0] ?? [];
    const shiftingLine = shifting?.geometry.coordinates[0] ?? [];

    expect(headLine[0]).toEqual(ROAD[0]);
    expect(shiftingLine[shiftingLine.length - 1]).toEqual(ROAD[ROAD.length - 1]);
    expect(headLine[headLine.length - 1]).toEqual(shiftingLine[0]);
  });

  it("dims the ground the chart is not showing, rather than dropping it", () => {
    show({ lit: [{ startIndex: 0, endIndex: 10 }] });

    expect(tintLayer()?.paint["line-opacity"]).toEqual(["case", ["get", "shown"], 1, 0.25]);
    expect(tintFeatures().map((feature) => feature.properties.shown)).toContain(false);
  });

  it("draws nothing at all when no stretch has a reading", () => {
    const { container } = show({ runs: [] });

    expect(tintLayer()).toBeUndefined();
    expect(container).toBeEmptyDOMElement();
  });
});

describe("what the tint says without the map", () => {
  it("names every stretch's wind in words", () => {
    show({});

    expect(screen.getByText(windRelationWords(0, 24))).toBeInTheDocument();
    expect(screen.getByText(windRelationWords(null, 24))).toBeInTheDocument();
    expect(screen.getByText(/headwind/)).toBeInTheDocument();
    expect(screen.getByText(/wind shifting/)).toBeInTheDocument();
  });

  it("gives the stretches a table of their own, named for what it reports", () => {
    show({});

    expect(
      screen.getByRole("table", { name: /Wind against the way you are riding/ }),
    ).toBeInTheDocument();
  });
});
