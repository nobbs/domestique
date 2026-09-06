/**
 * The custom layer, asked what it does to a graphics context that draws
 * nothing.
 *
 * jsdom has no WebGL, so the context here is a hand-written fake that records
 * the calls made to it. That answers nothing about what appears on the map — a
 * shader that fails to compile looks exactly like one that does from here — but
 * it does answer the two things this layer could get wrong without ever drawing
 * a wrong pixel: whether it survives being spread into `react-map-gl`, and
 * whether it gives back what it took when the map takes it away.
 */

import type { CustomLayerInterface, Map as MapLibreMap } from "maplibre-gl";
import { describe, expect, it, vi } from "vitest";
import { FLOATS_PER_VERTEX } from "../../lib/windField";
import type { StreakFrame } from "./windStreakLayer";
import { shaderColour, windStreakLayer } from "./windStreakLayer";

/** Every call the layer makes, in the order it made them. */
interface Recorder {
  gl: WebGL2RenderingContext;
  calls: Array<{ name: string; args: unknown[] }>;
  called: (name: string) => Array<unknown[]>;
}

const NAMES = [
  "createShader",
  "shaderSource",
  "compileShader",
  "getShaderParameter",
  "attachShader",
  "linkProgram",
  "getProgramParameter",
  "deleteShader",
  "getUniformLocation",
  "createBuffer",
  "createVertexArray",
  "bindVertexArray",
  "bindBuffer",
  "getAttribLocation",
  "enableVertexAttribArray",
  "vertexAttribPointer",
  "deleteProgram",
  "deleteBuffer",
  "deleteVertexArray",
  "useProgram",
  "uniformMatrix4fv",
  "uniform3f",
  "uniform1f",
  "bufferData",
  "enable",
  "disable",
  "isEnabled",
  "blendFunc",
  "drawArrays",
] as const;

/** Per-call return-value overrides, keyed by GL method name — for exercising failure paths. */
type Overrides = Partial<Record<(typeof NAMES)[number], unknown>>;

function recorder(overrides: Overrides = {}): Recorder {
  const calls: Array<{ name: string; args: unknown[] }> = [];
  const context: Record<string, unknown> = {
    VERTEX_SHADER: 1,
    FRAGMENT_SHADER: 2,
    ARRAY_BUFFER: 3,
    FLOAT: 4,
    DYNAMIC_DRAW: 5,
    BLEND: 6,
    DEPTH_TEST: 12,
    ONE: 7,
    ONE_MINUS_SRC_ALPHA: 8,
    LINES: 9,
    TRIANGLES: 13,
    COMPILE_STATUS: 10,
    LINK_STATUS: 11,
    createProgram: () => "program",
  };
  for (const name of NAMES) {
    context[name] = vi.fn((...args: unknown[]) => {
      calls.push({ name, args });
      if (name in overrides) {
        return overrides[name];
      }
      // A boolean method returning its own name would be truthy regardless of
      // what it is asked, which is exactly the bug this fake exists to catch.
      if (name === "isEnabled") {
        return false;
      }

      return name === "getAttribLocation" ? 0 : name;
    });
  }

  return {
    gl: context as unknown as WebGL2RenderingContext,
    calls,
    called: (name) => calls.filter((call) => call.name === name).map((call) => call.args),
  };
}

/** A frame of two streaks, with room in the buffer for three. */
function frame(
  vertexCount: number,
  primitive?: NonNullable<StreakFrame["primitive"]>,
): StreakFrame {
  const vertices = new Float32Array(6 * FLOATS_PER_VERTEX);
  vertices.fill(0.5);

  return {
    vertices,
    vertexCount,
    colour: [0.1, 0.2, 0.3],
    strength: 0.45,
    ...(primitive === undefined ? {} : { primitive }),
  };
}

/** The layer as MapLibre actually receives it: spread, never handed over whole. */
function spread(layer: CustomLayerInterface): CustomLayerInterface {
  return { ...layer };
}

const NO_MAP = null as unknown as MapLibreMap;

/**
 * The matrix for the 0..1 world square the field's vertices are in, in the
 * double precision `uniformMatrix4fv` refuses.
 */
const MERCATOR_MATRIX = new Float64Array(16).fill(1);

/**
 * The other matrix MapLibre offers, which is in world units and is what the
 * upstream custom-layer example still reaches for. Distinct from the one above
 * so a frame can be asked which of the two it actually uploaded.
 */
const WORLD_UNIT_MATRIX = new Float64Array(16).fill(2);

function draw(layer: CustomLayerInterface, gl: WebGL2RenderingContext) {
  layer.render(gl, {
    modelViewProjectionMatrix: WORLD_UNIT_MATRIX,
    defaultProjectionData: { mainMatrix: MERCATOR_MATRIX },
  } as never);
}

describe("the layer as a value", () => {
  it("names itself a custom layer, drawn flat on the map", () => {
    const layer = windStreakLayer("route-wind-field", () => frame(4));

    expect(layer.id).toBe("route-wind-field");
    expect(layer.type).toBe("custom");
    expect(layer.renderingMode).toBe("2d");
  });

  it("keeps every method through the spread `react-map-gl` puts it through", () => {
    const copy = spread(windStreakLayer("route-wind-field", () => frame(4)));

    expect(typeof copy.render).toBe("function");
    expect(typeof copy.onAdd).toBe("function");
    expect(typeof copy.onRemove).toBe("function");
  });
});

describe("what the layer asks of the context", () => {
  it("draws one line per streak, from the vertices this frame wrote", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([[context.gl.LINES, 0, 4]]);
    // Only what was written, never the whole buffer: the rest is last frame's.
    const [uploaded] = context.called("bufferData")[0]?.slice(1) ?? [];

    expect((uploaded as Float32Array).length).toBe(4 * FLOATS_PER_VERTEX);
  });

  it("draws triangles instead once the frame asks for them", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(6, "triangles")));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([[context.gl.TRIANGLES, 0, 6]]);
  });

  it("leaves the depth test off after drawing when it found it already off", () => {
    const context = recorder({ isEnabled: false });
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("disable")).toEqual([[context.gl.DEPTH_TEST]]);
    expect(context.called("enable")).toEqual([[context.gl.BLEND]]);
  });

  it("restores the depth test after drawing when it found it already on", () => {
    const context = recorder({ isEnabled: true });
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("enable")).toEqual([[context.gl.BLEND], [context.gl.DEPTH_TEST]]);
  });

  it("hands the matrix over as the single precision WebGL takes", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);
    const [, , matrix] = context.called("uniformMatrix4fv")[0] ?? [];

    expect(matrix).toBeInstanceOf(Float32Array);
  });

  // The one thing this fake context can say about where the field lands. The
  // two matrices differ by the world's pixel size, so taking the wrong one puts
  // every streak in a corner of the map and clips it away — with no GL error,
  // no failed compile and nothing on screen to notice.
  it("projects from the world square the vertices are in, not from world units", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);
    const [, , matrix] = context.called("uniformMatrix4fv")[0] ?? [];

    expect([...(matrix as Float32Array)]).toEqual([...MERCATOR_MATRIX]);
  });

  it("carries the colour and the strength the field is drawn at", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("uniform3f")[0]?.slice(1)).toEqual([0.1, 0.2, 0.3]);
    expect(context.called("uniform1f")[0]?.slice(1)).toEqual([0.45]);
  });

  it("draws nothing at all on a frame with no streaks on it", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(0)));
    layer.onAdd?.(NO_MAP, context.gl);

    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
    expect(context.called("useProgram")).toEqual([]);
  });

  it("draws nothing before it has been added to a map", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));

    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
  });

  it("gives back the program, the buffer and the array when the map lets it go", () => {
    const context = recorder();
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    layer.onRemove?.(NO_MAP, context.gl);

    expect(context.called("deleteProgram")).toHaveLength(1);
    expect(context.called("deleteBuffer")).toHaveLength(1);
    expect(context.called("deleteVertexArray")).toHaveLength(1);
    // And nothing is drawn with what has just been deleted.
    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
  });
});

describe("when the context refuses part of the setup", () => {
  it("deletes both shaders and draws nothing when one fails to compile", () => {
    const context = recorder({ getShaderParameter: false });
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    // One deleted before the program is even reached, one right after linking
    // never happens.
    expect(context.called("deleteShader")).toHaveLength(2);
    expect(context.called("deleteProgram")).toHaveLength(1);
    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
  });

  it("deletes the program and draws nothing when linking fails", () => {
    const context = recorder({ getProgramParameter: false });
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    expect(context.called("deleteProgram")).toHaveLength(1);
    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
  });

  it("cleans up the program and draws nothing when the buffer can't be allocated", () => {
    const context = recorder({ createBuffer: null });
    const layer = spread(windStreakLayer("route-wind-field", () => frame(4)));
    layer.onAdd?.(NO_MAP, context.gl);

    expect(context.called("deleteProgram")).toHaveLength(1);
    expect(context.called("deleteVertexArray")).toHaveLength(1);
    draw(layer, context.gl);

    expect(context.called("drawArrays")).toEqual([]);
  });
});

describe("a colour a shader can take", () => {
  it("splits a hex colour into its three components", () => {
    expect(shaderColour("#000000")).toEqual([0, 0, 0]);
    expect(shaderColour("#ffffff")).toEqual([1, 1, 1]);
    expect(shaderColour("#24282c")).toEqual([36 / 255, 40 / 255, 44 / 255]);
  });
});
