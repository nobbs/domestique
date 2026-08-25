import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Position } from "../../api/types";

interface SourceRecord {
  id: string;
  type: string;
  data: { features: Array<{ properties: { key: string } }> };
}

interface LayerRecord {
  id: string;
  filter?: unknown;
  paint: Record<string, unknown>;
}

const drawn = vi.hoisted(() => ({
  sources: [] as SourceRecord[],
  layers: [] as LayerRecord[],
}));

vi.mock("react-map-gl/maplibre", () => ({
  Source: ({ children, ...source }: { children?: ReactNode } & SourceRecord) => {
    drawn.sources.push(source);

    return <>{children}</>;
  },
  Layer: (layer: LayerRecord) => {
    drawn.layers.push(layer);

    return null;
  },
}));

const { LibraryRoutes, LIBRARY_HIT_LAYER } = await import("./LibraryRoutes");

beforeEach(() => {
  drawn.sources = [];
  drawn.layers = [];
});

function line(key: string, coordinates: Position[]) {
  return { key, coordinates };
}

function layer(id: string) {
  const found = drawn.layers.find((entry) => entry.id === id);
  if (!found) {
    throw new Error(`expected a ${id} layer`);
  }

  return found;
}

describe("LibraryRoutes", () => {
  it("draws selectable lines, including the transparent hit target", () => {
    render(
      <LibraryRoutes
        lines={[
          line("too-short", [[8, 49]]),
          line("selected", [
            [8, 49],
            [8.1, 49.1],
          ]),
          line("hovered", [
            [8.2, 49.2],
            [8.3, 49.3],
          ]),
        ]}
        selectedKey="selected"
        hoveredKey="hovered"
        hitLayerId={LIBRARY_HIT_LAYER}
      />,
    );

    expect(drawn.sources[0]?.data.features.map((feature) => feature.properties.key)).toEqual([
      "selected",
      "hovered",
    ]);
    expect(drawn.layers.map((entry) => entry.id)).toEqual([
      "library-line",
      "library-selected-line",
      "library-hover-line",
      LIBRARY_HIT_LAYER,
    ]);
    expect(layer("library-selected-line").filter).toEqual(["==", ["get", "key"], "selected"]);
    expect(layer("library-hover-line").filter).toEqual(["==", ["get", "key"], "hovered"]);
    expect(layer(LIBRARY_HIT_LAYER).paint).toMatchObject({
      "line-width": 18,
      "line-opacity": 0,
    });
  });
});
