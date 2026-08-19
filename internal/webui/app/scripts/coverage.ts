/**
 * The browser half of the UI coverage number, and the merge that makes it one
 * number.
 *
 * `vitest run --coverage` measures what jsdom can reach. The browser suite
 * reaches the rest — the MapLibre map, which needs WebGL, and the page-level
 * components that are assembled rather than unit-tested — and until this tool
 * existed none of it entered the LCOV file uploaded to Codecov. Those components
 * therefore reported low however thoroughly Chromium had just driven them, which
 * is why `patch.ui` could only report and never gate.
 *
 * What runs here, in order:
 *
 *   1. the `dev-server` Playwright project, with `e2e/coverage.ts` recording V8
 *      coverage for every page it opens;
 *   2. each recorded module mapped back through the dev server's inline source
 *      map to the file in `src/` it was built from;
 *   3. that, merged with the Vitest report and written back over `lcov.info`.
 *
 * Both halves are V8 coverage — Vitest's provider is `v8`, and Chromium reports
 * in the same format — so the merge aligns statement for statement instead of
 * approximating one against the other. A statement both suites reached is one
 * statement in the result, which is what keeps the total honest.
 *
 * The subject is taken from the Vitest report rather than restated here. Every
 * file the coverage `include` matches appears in it, covered or not, so
 * intersecting against its file list is exactly the `include` and `exclude` in
 * `vite.config.ts` — and it cannot drift from them, because it is them. That
 * also drops `src/main.tsx`, which the browser does load and Vitest deliberately
 * does not measure: counted on one side only it would add statements no Vitest
 * run can ever reach, and the merged number would depend on which collector ran
 * rather than on the tree. The same holds one level down, per statement rather
 * than per file, which is what `align` below is for.
 */

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, readdir, readFile, rm } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { chromium } from "@playwright/test";
import convert, { type Options } from "ast-v8-to-istanbul";
// The three istanbul packages are CommonJS, and Node runs this file as ESM.
// Named imports out of a CommonJS module work only where Node's lexer manages to
// guess the names, which it does not for `createCoverageMap`, so take the whole
// of `module.exports` from each and reach through it.
import type { CoverageMap, CoverageMapData, FileCoverageData } from "istanbul-lib-coverage";
import libCoverage from "istanbul-lib-coverage";
import libReport from "istanbul-lib-report";
import reports from "istanbul-reports";
import { parseAstAsync } from "vite";

/** A source map as the converter wants one. */
type SourceMap = NonNullable<Options["sourceMap"]>;

/** One module's V8 coverage, as `e2e/coverage.ts` recorded it. */
export interface RecordedModule {
  /** The dev server URL the module was served from. */
  url: string;
  /** The module text as the browser executed it, which the V8 ranges index into. */
  source: string | undefined;
  /** V8's own per-function range coverage. */
  functions: Options["coverage"]["functions"];
}

const appDirectory = path.resolve(import.meta.dirname, "..");
const reportDirectory = path.resolve(appDirectory, "../../..", ".coverage", "ui");
const recordingDirectory = path.join(reportDirectory, "browser");
const vitestReport = path.join(reportDirectory, "coverage-final.json");

/**
 * The dev server appends its source map to every module it serves.
 *
 * Nothing else can attribute a V8 range: the browser executes the transformed
 * module, and the file the number is about is the one it was transformed from.
 */
const INLINE_SOURCE_MAP =
  /\/\/# sourceMappingURL=data:application\/json;(?:charset=[^;]+;)?base64,([A-Za-z0-9+/=]+)\s*$/;

/** Reads back the source map the dev server appended, if it appended one. */
export function inlineSourceMap(source: string): SourceMap | undefined {
  const match = INLINE_SOURCE_MAP.exec(source);
  if (match?.[1] === undefined) {
    return undefined;
  }

  return JSON.parse(Buffer.from(match[1], "base64").toString("utf8")) as SourceMap;
}

/**
 * The working-tree file a served module was built from.
 *
 * A dev server URL names the module's own path and the map names its original
 * file relative to that, so the two together are the answer: `/src/api/client.ts`
 * with a source of `client.ts` is `src/api/client.ts`. A query string — the
 * dev server's cache buster — is not part of it.
 */
export function sourcePath(
  url: string,
  sourceMap: SourceMap,
  directory: string,
): string | undefined {
  const source = sourceMap.sources[0];
  if (source === undefined || source === null) {
    return undefined;
  }
  const served = new URL(url).pathname;

  return path.resolve(directory, `.${path.posix.join(path.posix.dirname(served), source)}`);
}

/**
 * Copies coverage data, deeply.
 *
 * Through JSON rather than `structuredClone`, because a map's values are plain
 * data when they were parsed from a report and `FileCoverage` instances when
 * they came out of a `CoverageMap`. Serialising accepts either — an instance
 * writes itself out as its data — where a structured clone would turn the second
 * kind into an object wrapping it, which nothing downstream recognises.
 */
function copy(data: CoverageMapData): CoverageMapData {
  return JSON.parse(JSON.stringify(data)) as CoverageMapData;
}

/** Indexes a statement, function or branch map by the location identifying each entry. */
function byLocation<Entry>(
  map: Record<string, Entry>,
  where: (entry: Entry) => unknown,
): Map<string, string> {
  const index = new Map<string, string>();
  for (const [key, entry] of Object.entries(map)) {
    index.set(JSON.stringify(where(entry)), key);
  }

  return index;
}

/**
 * Restates the browser's coverage of one file in the terms the Vitest report
 * uses for that same file.
 *
 * Both collectors instrument the same source, but through different transforms —
 * Vitest's, for jsdom, and Vite's, for the browser — and the statement, function
 * and branch boundaries the two derive are very nearly, but not exactly, the
 * same. Merged as they arrive, the handful that differ are taken for entries the
 * other side had never seen and appended, and the file ends up reporting more
 * statements than it has.
 *
 * So the browser's counts are carried across onto Vitest's own maps, matched by
 * source location, and anything with no counterpart there is dropped. The
 * denominator is then exactly the one the Vitest report set, whatever the two
 * transforms disagree about, and merging can only move the numerator.
 */
function align(vitest: FileCoverageData, browser: FileCoverageData): FileCoverageData {
  const statements = byLocation(browser.statementMap, (entry) => entry);
  const s: FileCoverageData["s"] = {};
  for (const [key, entry] of Object.entries(vitest.statementMap)) {
    const from = statements.get(JSON.stringify(entry));
    s[key] = from === undefined ? 0 : (browser.s[from] ?? 0);
  }

  // A function is identified by its declaration rather than its body, which is
  // what tells two closures written on the same line apart.
  const functions = byLocation(browser.fnMap, (entry) => entry.decl);
  const f: FileCoverageData["f"] = {};
  for (const [key, entry] of Object.entries(vitest.fnMap)) {
    const from = functions.get(JSON.stringify(entry.decl));
    f[key] = from === undefined ? 0 : (browser.f[from] ?? 0);
  }

  // By the branch's arms rather than the expression around them: the arms are
  // what the counts are per, so matching on them keeps the two aligned.
  const branches = byLocation(browser.branchMap, (entry) => entry.locations);
  const b: FileCoverageData["b"] = {};
  for (const [key, entry] of Object.entries(vitest.branchMap)) {
    const from = branches.get(JSON.stringify(entry.locations));
    const counts = from === undefined ? undefined : browser.b[from];
    b[key] = entry.locations.map((_, arm) => counts?.[arm] ?? 0);
  }

  return {
    path: vitest.path,
    statementMap: vitest.statementMap,
    fnMap: vitest.fnMap,
    branchMap: vitest.branchMap,
    s,
    f,
    b,
  };
}

/**
 * Merges what the browser reached into what Vitest measured.
 *
 * The Vitest report is the subject on both axes: which files are measured, and
 * which statements each of them has. A file the report does not name is dropped
 * whole, and what survives is aligned onto that file's own maps. Both sides are
 * copied first because `merge` writes through to the coverage it is given, and a
 * caller that then read its own report back would find it had changed underneath.
 */
export function mergeCoverage(vitest: CoverageMapData, browser: CoverageMapData): CoverageMap {
  const subject = copy(vitest);
  const measured: CoverageMapData = {};
  for (const [file, coverage] of Object.entries(copy(browser))) {
    const known = subject[file];
    if (known !== undefined) {
      measured[file] = align(known, coverage);
    }
  }

  const merged = libCoverage.createCoverageMap(copy(vitest));
  merged.merge(libCoverage.createCoverageMap(measured));

  return merged;
}

/** Turns one recorded module into Istanbul coverage of the file it came from. */
async function convertModule(
  recorded: RecordedModule,
  directory: string,
): Promise<CoverageMapData | undefined> {
  if (recorded.source === undefined) {
    return undefined;
  }
  const sourceMap = inlineSourceMap(recorded.source);
  if (sourceMap === undefined) {
    return undefined;
  }
  const file = sourcePath(recorded.url, sourceMap, directory);
  if (file === undefined) {
    return undefined;
  }

  return await convert({
    code: recorded.source,
    sourceMap,
    ast: await parseAstAsync(recorded.source),
    // The converter reads this back with `fileURLToPath`, so it wants a URL and
    // not the path it will turn into.
    coverage: { url: pathToFileURL(file).href, functions: recorded.functions },
  });
}

/** Everything the browser suite reached, as one coverage map. */
export async function collectRecorded(directory: string): Promise<CoverageMap> {
  const collected = libCoverage.createCoverageMap({});
  const files = (await readdir(directory)).filter((name) => name.endsWith(".json"));

  for (const name of files) {
    const recorded = JSON.parse(
      await readFile(path.join(directory, name), "utf8"),
    ) as RecordedModule[];
    for (const entry of recorded) {
      const converted = await convertModule(entry, appDirectory);
      if (converted !== undefined) {
        collected.merge(converted);
      }
    }
  }

  return collected;
}

/** Writes the LCOV file Codecov reads, in place of the one Vitest wrote. */
function writeLcov(coverageMap: CoverageMap): void {
  const context = libReport.createContext({ dir: reportDirectory, coverageMap });
  // The reporter names each file relative to `projectRoot`, which defaults to
  // the working directory. Naming it explicitly is what keeps the paths the same
  // as the ones Vitest wrote — `src/App.tsx`, which the `fixes` rule in
  // codecov.yml maps back to the repository — however this file was invoked.
  reports.create("lcovonly", { file: "lcov.info", projectRoot: appDirectory }).execute(context);
}

/** The share of statements a coverage map reports as reached. */
function percentage(coverageMap: CoverageMap): string {
  const statements = coverageMap.getCoverageSummary().toJSON().statements;

  return `${statements.pct}% (${statements.covered}/${statements.total})`;
}

function say(message: string): void {
  process.stdout.write(`${message}\n`);
}

async function run(): Promise<number> {
  if (!existsSync(vitestReport)) {
    process.stderr.write(
      `no Vitest report at ${vitestReport}: run \`make coverage-ui\`, which measures that half first\n`,
    );

    return 1;
  }

  // A contributor who has never run the browser suite has no browser, and
  // downloading one is a deliberate act rather than something a coverage run
  // should do behind their back. Report the half that was measured and say which
  // half was not, rather than failing or quietly reporting a lower number than
  // CI does for the same tree.
  if (!existsSync(chromium.executablePath())) {
    process.stderr.write(
      "no browser installed, so the LCOV report covers the Vitest suites only.\n" +
        "The map and the page-level components will read lower here than they do in CI.\n" +
        "Run `make ui-browser-install` to measure the browser half as well.\n",
    );

    return 0;
  }

  await rm(recordingDirectory, { recursive: true, force: true });
  await mkdir(recordingDirectory, { recursive: true });

  // Only `dev-server`. The `bundle` project drives minified `vite build` output
  // with no source map, so its ranges cannot be attributed back to `src/**`; see
  // the note in e2e/coverage.ts.
  const suite = spawnSync(
    path.join(appDirectory, "node_modules", ".bin", "playwright"),
    ["test", "--project=dev-server"],
    {
      cwd: appDirectory,
      stdio: "inherit",
      env: { ...process.env, WEBUI_COVERAGE_DIR: recordingDirectory },
    },
  );
  if (suite.status !== 0) {
    process.stderr.write("the browser suite failed, so its coverage describes nothing\n");

    return suite.status ?? 1;
  }

  const vitest = JSON.parse(await readFile(vitestReport, "utf8")) as CoverageMapData;
  const browser = await collectRecorded(recordingDirectory);
  const merged = mergeCoverage(vitest, browser.toJSON());
  writeLcov(merged);

  say("");
  say("% Coverage report from the browser suite");
  say("");
  say("=============================== Merged coverage ================================");
  say(`Vitest suites   : ${percentage(libCoverage.createCoverageMap(copy(vitest)))}`);
  say(`Browser suite   : ${percentage(browser)}`);
  say(`Merged          : ${percentage(merged)}`);
  say("================================================================================");

  return 0;
}

if (import.meta.main) {
  process.exitCode = await run();
}
