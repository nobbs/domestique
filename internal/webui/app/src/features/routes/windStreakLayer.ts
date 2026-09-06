/**
 * The wind field's streaks, drawn inside MapLibre's own render pass.
 *
 * A MapLibre custom layer rather than a canvas over the map, for one reason:
 * ordering. A canvas is a sibling of the map's own canvas and sits above every
 * layer in it, so the field would be drawn over the route line the reader is
 * following. A custom layer takes a `beforeId` like any other, so the field
 * goes under the route exactly where the wash does.
 *
 * The only WebGL in this application. Everything above it — where a streak is,
 * how strongly it is drawn — is decided in `windField.ts` and arrives here as a
 * buffer of vertices; this owns a program, a buffer and nothing else. MapLibre
 * unbinds its vertex array before calling `render` and marks its own state
 * dirty afterwards, so the program and buffer bindings here cannot leak into
 * the map's drawing.
 *
 * Mercator only. `defaultProjectionData.mainMatrix` is the one matrix MapLibre
 * hands a custom layer that takes the 0..1 world square `mercatorXY` writes;
 * `modelViewProjectionMatrix`, which the upstream example still passes, is in
 * world units — mercator times the world's pixel size — so feeding it these
 * coordinates collapses the whole field into a corner of the map and clips it
 * away without an error. A globe projection would need the shader prelude
 * MapLibre offers instead, and no map here sets one.
 */

import type { CustomLayerInterface } from "maplibre-gl";
import { FLOATS_PER_VERTEX } from "../../lib/windField";

const VERTEX_SOURCE = `#version 300 es
uniform mat4 u_matrix;
in vec2 a_position;
in float a_alpha;
out float v_alpha;
void main() {
  v_alpha = a_alpha;
  gl_Position = u_matrix * vec4(a_position, 0.0, 1.0);
}`;

const FRAGMENT_SOURCE = `#version 300 es
precision mediump float;
uniform vec3 u_colour;
uniform float u_strength;
in float v_alpha;
out vec4 fragColour;
void main() {
  // Premultiplied: the translucent pass blends ONE, ONE_MINUS_SRC_ALPHA.
  float alpha = v_alpha * u_strength;
  fragColour = vec4(u_colour * alpha, alpha);
}`;

/** A colour as the three components a shader takes, from `#rrggbb`. */
export function shaderColour(hex: string): [number, number, number] {
  const value = Number.parseInt(hex.replace("#", ""), 16);

  return Number.isFinite(value)
    ? [((value >> 16) & 255) / 255, ((value >> 8) & 255) / 255, (value & 255) / 255]
    : [1, 1, 1];
}

/** What the layer draws on the frame it is asked to draw. */
export interface StreakFrame {
  /** Interleaved as `windField.ts` writes them: mercator x, mercator y, alpha. */
  vertices: Float32Array;
  /** How many of them are this frame's, which is what `writeStreaks` returned. */
  vertexCount: number;
  colour: readonly [number, number, number];
  /** How strongly the whole field is drawn, over the alpha each streak carries. */
  strength: number;
  /**
   * Hairlines by default; `"triangles"` for a caller that writes each streak as
   * a quad, since WebGL draws every line one device pixel wide whatever is asked.
   */
  primitive?: "lines" | "triangles";
}

function compile(gl: WebGL2RenderingContext, kind: number, source: string): WebGLShader | null {
  const shader = gl.createShader(kind);
  if (!shader) {
    return null;
  }
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    gl.deleteShader(shader);

    return null;
  }

  return shader;
}

/**
 * The layer, reading each frame from the caller at the moment it draws.
 *
 * A plain object with its methods as own properties, never a class instance:
 * `react-map-gl` spreads the layer it is handed before adding it, and a spread
 * of an instance leaves every prototype method behind.
 */
export function windStreakLayer(id: string, frame: () => StreakFrame): CustomLayerInterface {
  let program: WebGLProgram | null = null;
  let buffer: WebGLBuffer | null = null;
  let vertexArray: WebGLVertexArrayObject | null = null;
  let matrixLocation: WebGLUniformLocation | null = null;
  let colourLocation: WebGLUniformLocation | null = null;
  let strengthLocation: WebGLUniformLocation | null = null;
  // Copied into rather than uploaded straight: MapLibre's matrix may be double
  // precision, which `uniformMatrix4fv` refuses.
  const matrix = new Float32Array(16);
  const stride = FLOATS_PER_VERTEX * Float32Array.BYTES_PER_ELEMENT;

  return {
    id,
    type: "custom",
    renderingMode: "2d",

    onAdd(_map, gl) {
      const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX_SOURCE);
      const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SOURCE);
      program = gl.createProgram();
      if (!program || !vertex || !fragment) {
        if (vertex) {
          gl.deleteShader(vertex);
        }
        if (fragment) {
          gl.deleteShader(fragment);
        }
        if (program) {
          gl.deleteProgram(program);
        }
        program = null;

        return;
      }
      gl.attachShader(program, vertex);
      gl.attachShader(program, fragment);
      gl.linkProgram(program);
      gl.deleteShader(vertex);
      gl.deleteShader(fragment);
      if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
        gl.deleteProgram(program);
        program = null;

        return;
      }
      matrixLocation = gl.getUniformLocation(program, "u_matrix");
      colourLocation = gl.getUniformLocation(program, "u_colour");
      strengthLocation = gl.getUniformLocation(program, "u_strength");

      buffer = gl.createBuffer();
      vertexArray = gl.createVertexArray();
      if (!buffer || !vertexArray) {
        gl.deleteProgram(program);
        if (buffer) {
          gl.deleteBuffer(buffer);
        }
        if (vertexArray) {
          gl.deleteVertexArray(vertexArray);
        }
        program = null;
        buffer = null;
        vertexArray = null;

        return;
      }
      gl.bindVertexArray(vertexArray);
      gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
      const position = gl.getAttribLocation(program, "a_position");
      const alpha = gl.getAttribLocation(program, "a_alpha");
      gl.enableVertexAttribArray(position);
      gl.vertexAttribPointer(position, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(alpha);
      gl.vertexAttribPointer(alpha, 1, gl.FLOAT, false, stride, 2 * Float32Array.BYTES_PER_ELEMENT);
      gl.bindVertexArray(null);
    },

    onRemove(_map, gl) {
      if (program) {
        gl.deleteProgram(program);
      }
      if (buffer) {
        gl.deleteBuffer(buffer);
      }
      if (vertexArray) {
        gl.deleteVertexArray(vertexArray);
      }
      program = null;
      buffer = null;
      vertexArray = null;
    },

    render(gl, options) {
      const { vertices, vertexCount, colour, strength, primitive } = frame();
      if (!program || !vertexArray || !buffer || vertexCount === 0) {
        return;
      }
      matrix.set(options.defaultProjectionData.mainMatrix);
      // biome-ignore lint/correctness/useHookAtTopLevel: `gl.useProgram` is WebGL's, not React's; the rule matches it on its name alone.
      gl.useProgram(program);
      gl.uniformMatrix4fv(matrixLocation, false, matrix);
      gl.uniform3f(colourLocation, colour[0], colour[1], colour[2]);
      gl.uniform1f(strengthLocation, strength);
      gl.bindVertexArray(vertexArray);
      gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        vertices.subarray(0, vertexCount * FLOATS_PER_VERTEX),
        gl.DYNAMIC_DRAW,
      );
      // MapLibre hands a 2D custom layer its last depth range and test still
      // on, and a streak at z=0 sits at the far edge of it and fails LEQUAL.
      // Restored afterwards: a later layer in the same pass is entitled to find
      // the depth test the way MapLibre left it, not the way this one leaves it.
      const depthWasEnabled = gl.isEnabled(gl.DEPTH_TEST);
      gl.disable(gl.DEPTH_TEST);
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      gl.drawArrays(primitive === "triangles" ? gl.TRIANGLES : gl.LINES, 0, vertexCount);
      if (depthWasEnabled) {
        gl.enable(gl.DEPTH_TEST);
      }
      gl.bindVertexArray(null);
    },
  };
}
