/**
 * The parts of the coverage merge that a wrong answer would be invisible in.
 *
 * A merge that double-counted would not fail anything: it would report a total
 * that looks plausible and a percentage that is quietly wrong, and the patch
 * status derived from it would pass or fail for reasons nobody could reproduce.
 * The same goes for attribution — a module mapped to the wrong path simply does
 * not appear, and the file it belonged to keeps reading as untested.
 */

import type { CoverageMapData } from "istanbul-lib-coverage";
import libCoverage from "istanbul-lib-coverage";
import { describe, expect, it } from "vitest";
import { inlineSourceMap, mergeCoverage, sourcePath } from "./coverage.ts";

const APP = "/repo/internal/webui/app";

/** One file's coverage over `count` statements, with `hits` naming the reached ones. */
function fileCoverage(file: string, count: number, hits: number[]): CoverageMapData {
  const statementMap: CoverageMapData[string]["statementMap"] = {};
  const s: CoverageMapData[string]["s"] = {};
  for (let index = 0; index < count; index += 1) {
    statementMap[index] = {
      start: { line: index + 1, column: 0 },
      end: { line: index + 1, column: 20 },
    };
    s[index] = hits.includes(index) ? 1 : 0;
  }

  return { [file]: { path: file, statementMap, fnMap: {}, branchMap: {}, s, f: {}, b: {} } };
}

function statements(data: ReturnType<typeof mergeCoverage>) {
  return data.getCoverageSummary().toJSON().statements;
}

describe("mergeCoverage", () => {
  const file = `${APP}/src/features/routes/RouteDetail.tsx`;

  it("counts a statement both suites reached exactly once", () => {
    // Statement 1 is reached by both. Added rather than merged, the total would
    // be 8 and the covered count 4; the honest answer is 4 statements, 3 of them
    // reached.
    const vitest = fileCoverage(file, 4, [0, 1]);
    const browser = fileCoverage(file, 4, [1, 2]);

    const merged = statements(mergeCoverage(vitest, browser));

    expect(merged.total).toBe(4);
    expect(merged.covered).toBe(3);
  });

  it("keeps the denominator the Vitest report set", () => {
    const vitest = fileCoverage(file, 10, []);
    const browser = fileCoverage(file, 10, [0, 1, 2]);

    expect(statements(mergeCoverage(vitest, browser)).total).toBe(10);
  });

  it("adds what only the browser reached", () => {
    const vitest = fileCoverage(file, 4, [0]);
    const browser = fileCoverage(file, 4, [3]);

    expect(statements(mergeCoverage(vitest, browser)).covered).toBe(2);
  });

  it("ignores a file the Vitest report does not measure", () => {
    // src/main.tsx is excluded deliberately, and the browser loads it on every
    // page. Counted here it would add statements no Vitest run can reach.
    const vitest = fileCoverage(`${APP}/src/App.tsx`, 2, [0]);
    const browser = fileCoverage(`${APP}/src/main.tsx`, 6, [0, 1, 2, 3, 4, 5]);

    const merged = mergeCoverage(vitest, browser);

    expect(merged.files()).toEqual([`${APP}/src/App.tsx`]);
    expect(statements(merged).total).toBe(2);
  });

  it("ignores a statement at a location the Vitest report's map does not have", () => {
    // The two transforms derive very nearly the same statement boundaries, and
    // the few that differ would otherwise arrive as statements the file does not
    // have — inflating its denominator with something no Vitest run can reach.
    const vitest = fileCoverage(file, 4, []);
    const browser: CoverageMapData = {
      [file]: {
        path: file,
        statementMap: {
          0: { start: { line: 1, column: 0 }, end: { line: 1, column: 20 } },
          1: { start: { line: 2, column: 4 }, end: { line: 2, column: 9 } },
        },
        fnMap: {},
        branchMap: {},
        s: { 0: 1, 1: 7 },
        f: {},
        b: {},
      },
    };

    const merged = statements(mergeCoverage(vitest, browser));

    expect(merged.total).toBe(4);
    expect(merged.covered).toBe(1);
  });

  it("carries functions and branches over by location rather than by key", () => {
    // The two collectors number their entries independently, so the same
    // function is entry 0 on one side and entry 7 on the other.
    const decl = { start: { line: 1, column: 0 }, end: { line: 1, column: 10 } };
    const loc = { start: { line: 2, column: 0 }, end: { line: 2, column: 30 } };
    const arms = [
      { start: { line: 2, column: 10 }, end: { line: 2, column: 20 } },
      { start: { line: 2, column: 21 }, end: { line: 2, column: 30 } },
    ];
    const measured = (key: number, fn: number, branch: number[]): CoverageMapData => ({
      [file]: {
        path: file,
        statementMap: {},
        s: {},
        fnMap: { [key]: { name: "render", decl, loc, line: 1 } },
        f: { [key]: fn },
        branchMap: { [key]: { loc, type: "binary-expr", locations: arms, line: 2 } },
        b: { [key]: branch },
      },
    });

    const merged = mergeCoverage(measured(0, 0, [0, 0]), measured(7, 3, [1, 2]))
      .getCoverageSummary()
      .toJSON();

    expect(merged.functions).toMatchObject({ total: 1, covered: 1 });
    expect(merged.branches).toMatchObject({ total: 2, covered: 2 });
  });

  it("accepts a browser side that came out of a coverage map", () => {
    // Which is how it arrives: `collectRecorded` returns a `CoverageMap`, and a
    // map holds `FileCoverage` instances where a parsed report holds plain data.
    const vitest = fileCoverage(file, 4, [0]);
    const browser = libCoverage.createCoverageMap(fileCoverage(file, 4, [1])).toJSON();

    expect(statements(mergeCoverage(vitest, browser)).covered).toBe(2);
  });

  it("leaves both reports as it found them", () => {
    // `merge` writes through to the coverage it is given, so a caller that read
    // its own report back afterwards would find the numbers had moved.
    const vitest = fileCoverage(file, 4, [0]);
    const browser = fileCoverage(file, 4, [1]);
    const before = structuredClone(vitest);

    mergeCoverage(vitest, browser);

    expect(vitest).toEqual(before);
  });
});

describe("sourcePath", () => {
  it("resolves a served module against the map's own source", () => {
    const map = { version: 3, sources: ["client.ts"], names: [], mappings: "" };

    expect(sourcePath("http://localhost:5173/src/api/client.ts", map, APP)).toBe(
      `${APP}/src/api/client.ts`,
    );
  });

  it("ignores the dev server's cache buster", () => {
    const map = { version: 3, sources: ["RouteDetail.tsx"], names: [], mappings: "" };

    expect(
      sourcePath("http://localhost:5173/src/features/routes/RouteDetail.tsx?t=1730000", map, APP),
    ).toBe(`${APP}/src/features/routes/RouteDetail.tsx`);
  });

  it("has no answer for a map that names no source", () => {
    expect(sourcePath("http://localhost:5173/src/a.ts", { sources: [] } as never, APP)).toBe(
      undefined,
    );
  });
});

describe("inlineSourceMap", () => {
  const map = { version: 3, sources: ["a.ts"], names: [], mappings: "AAAA" };
  // Assembled rather than written out: a literal one in this file is a source
  // map comment as far as every tool that reads this file is concerned, and Vite
  // tries to load it while collecting the suite.
  const marker = `//# ${"sourceMappingURL"}=data:application/json`;
  const encoded = Buffer.from(JSON.stringify(map)).toString("base64");
  const appended = `const a = 1;\n${marker};base64,${encoded}\n`;

  it("reads back the map the dev server appended", () => {
    expect(inlineSourceMap(appended)?.sources).toEqual(["a.ts"]);
  });

  it("reads back a map the dev server labelled with a charset", () => {
    expect(
      inlineSourceMap(appended.replace("json;base64", "json;charset=utf-8;base64"))?.sources,
    ).toEqual(["a.ts"]);
  });

  it("has no answer for a module served without one", () => {
    expect(inlineSourceMap("const a = 1;\n")).toBe(undefined);
  });
});
