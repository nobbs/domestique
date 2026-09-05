/**
 * The scalar wash, asked whether it hands the map a freshly-drawn image and
 * cleans up after itself.
 *
 * jsdom draws nothing, so what this can answer is not what the raster looks
 * like but the two things `toDataURL` was replaced for: that the source it
 * hands MapLibre is a blob URL rather than a giant base64 string, and that
 * every URL this component creates is revoked once it is done with it —
 * by the next grid, or by the overlay leaving the map.
 */

import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MEASURES } from "../../lib/measures";
import type { ScalarGrid } from "../../lib/windGrid";

interface SourceRecord {
  id: string;
  url: string;
  coordinates: unknown;
}

const seen = vi.hoisted(() => ({ sources: [] as SourceRecord[] }));

vi.mock("react-map-gl/maplibre", () => ({
  Source: (props: SourceRecord & { children?: unknown }) => {
    seen.sources.push({ id: props.id, url: props.url, coordinates: props.coordinates });

    return null;
  },
  Layer: () => null,
}));

vi.mock("../../components/map/CartographyContext", () => ({
  useCartography: () => ({ dark: false }),
}));

const gridState = vi.hoisted(() => ({ data: null as ScalarGrid | null }));

vi.mock("./useViewportGrid", () => ({
  useViewportGrid: () => ({ data: gridState.data, bboxRef: { current: null } }),
}));

vi.mock("../../api/openMeteoGrid", () => ({
  scalarGridReader: () => async () => null,
}));

const { ScalarOverlay } = await import("./ScalarOverlay");

const TEMPERATURE = MEASURES.find((measure) => measure.key === "temperature");
if (!TEMPERATURE) {
  throw new Error("temperature missing from MEASURES");
}

/** A 2x2 grid, warm enough everywhere to land in a real band. */
const GRID: ScalarGrid = {
  lonMin: 7,
  latMin: 48,
  dx: 0.02,
  dy: 0.02,
  nx: 2,
  ny: 2,
  values: new Float32Array([20, 20, 20, 20]),
};

let revoked: string[] = [];
let created = 0;

beforeEach(() => {
  seen.sources = [];
  gridState.data = null;
  revoked = [];
  created = 0;

  // jsdom has no 2D context; a minimal fake carries the drawing calls
  // `ScalarOverlay` makes through to a `toBlob` that resolves asynchronously,
  // the same as a real canvas.
  HTMLCanvasElement.prototype.getContext = vi.fn(() => ({
    createImageData: (width: number, height: number) => ({
      data: new Uint8ClampedArray(width * height * 4),
    }),
    putImageData: () => {},
  })) as unknown as typeof HTMLCanvasElement.prototype.getContext;
  HTMLCanvasElement.prototype.toBlob = vi.fn((callback: BlobCallback) => {
    setTimeout(() => callback(new Blob()), 0);
  });
  URL.createObjectURL = vi.fn(() => `blob:fake-${created++}`);
  URL.revokeObjectURL = vi.fn((url: string) => {
    revoked.push(url);
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ScalarOverlay", () => {
  it("hands the map a blob URL once the raster has been painted, not a data URL", async () => {
    gridState.data = GRID;
    const { rerender } = render(
      <ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />,
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);

    const source = seen.sources.at(-1);

    expect(source?.url).toBe("blob:fake-0");
  });

  it("revokes the previous blob's URL once a new grid replaces it", async () => {
    gridState.data = GRID;
    const { rerender } = render(
      <ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />,
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // A different grid, which is what a new viewport or a scrubbed hour hands
    // the component: a new object identity even where nothing else lines up.
    gridState.data = { ...GRID, values: new Float32Array([21, 21, 21, 21]) };
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(revoked).toEqual(["blob:fake-0"]);
  });

  it("revokes its blob's URL once switched off, drawing nothing more", async () => {
    gridState.data = GRID;
    const { rerender } = render(
      <ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />,
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // What the real `useViewportGrid` reports once `on` goes false: the query
    // is disabled and its data goes with it, which is what actually drives
    // this component's own cleanup — `on` alone changes nothing it reads.
    const sourcesBefore = seen.sources.length;
    gridState.data = null;
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={false} />);

    expect(revoked).toEqual(["blob:fake-0"]);
    // No further `<Source>` render: the component returned null instead.
    expect(seen.sources.length).toBe(sourcesBefore);
  });

  it("stops pointing at the old blob once a canvas cannot get a 2D context", async () => {
    gridState.data = GRID;
    const { rerender } = render(
      <ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />,
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(seen.sources.at(-1)?.url).toBe("blob:fake-0");

    // The next grid's canvas fails to hand back a context, the way an
    // exhausted WebGL/canvas context budget would in a real browser. Cleanup
    // from the render above has already revoked "blob:fake-0" by this point.
    HTMLCanvasElement.prototype.getContext = vi.fn(
      () => null,
    ) as unknown as typeof HTMLCanvasElement.prototype.getContext;
    gridState.data = { ...GRID, values: new Float32Array([21, 21, 21, 21]) };
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);
    // React renders once synchronously on the new props before this effect
    // runs, which still carries yesterday's `image` state — normal ordering,
    // not the bug, and indistinguishable at this point from the fixed
    // behaviour: nothing pushes a further `<Source>` yet either way.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // What actually tells the two apart: a render for any other reason,
    // once the failed effect has run. `seen.sources` only ever grows, so a
    // stale `image` and a cleared one look the same until something forces
    // one more render — unfixed, `image` never changed, so this still
    // carries the revoked "blob:fake-0" into one more `<Source>` push; fixed,
    // `image` is null and the component renders nothing, adding no entry.
    const sourcesBeforeExtraRender = seen.sources.length;
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);

    expect(seen.sources.length).toBe(sourcesBeforeExtraRender);
  });

  it("stops pointing at the old blob once toBlob calls back with nothing", async () => {
    gridState.data = GRID;
    const { rerender } = render(
      <ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />,
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(seen.sources.at(-1)?.url).toBe("blob:fake-0");

    // The next grid's encode fails, the way a canvas too large for the
    // platform's own limits would. Cleanup from the render above has already
    // revoked "blob:fake-0" by this point.
    HTMLCanvasElement.prototype.toBlob = vi.fn((callback: BlobCallback) => {
      setTimeout(() => callback(null), 0);
    });
    gridState.data = { ...GRID, values: new Float32Array([21, 21, 21, 21]) };
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    // What actually tells the two apart: a render for any other reason,
    // once the failed effect has run. `seen.sources` only ever grows, so a
    // stale `image` and a cleared one look the same until something forces
    // one more render — unfixed, `image` never changed, so this still
    // carries the revoked "blob:fake-0" into one more `<Source>` push; fixed,
    // `image` is null and the component renders nothing, adding no entry.
    const sourcesBeforeExtraRender = seen.sources.length;
    rerender(<ScalarOverlay measure={TEMPERATURE} variable="temperature_2m" on={true} />);

    expect(seen.sources.length).toBe(sourcesBeforeExtraRender);
  });
});
