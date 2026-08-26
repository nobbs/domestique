/**
 * Opens every story and docs page in a built Storybook and fails on any that
 * renders as an error.
 *
 * The Vitest projects cannot answer this question. Both run under the test
 * runner, so a story that reaches for something only the runner provides — the
 * `vi` object was the one that bit us — passes there and shows an error card to
 * anyone who opens Storybook itself. Three stories sat broken that way with
 * every check green. This reads the pages the way a reader does.
 *
 * Run against `storybook-static`, not the dev server: it is what a reader is
 * eventually served, and it needs no bundler running beside it.
 */

import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";
import process from "node:process";
import { chromium } from "playwright";

const ROOT = new URL("./storybook-static/", import.meta.url).pathname;
const TYPES = {
  ".css": "text/css",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".json": "application/json",
  ".map": "application/json",
  ".mjs": "text/javascript",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
};

/** Serves the built Storybook, and nothing above it. */
function serve() {
  const server = createServer((request, response) => {
    const asked = normalize(decodeURIComponent(new URL(request.url, "http://x").pathname));
    const path = join(ROOT, asked === "/" ? "index.html" : asked);
    if (!path.startsWith(ROOT)) {
      response.writeHead(403).end();

      return;
    }
    stat(path).then(
      (info) => {
        if (!info.isFile()) {
          response.writeHead(404).end();

          return;
        }
        response.writeHead(200, { "content-type": TYPES[extname(path)] ?? "application/octet-stream" });
        createReadStream(path).pipe(response);
      },
      () => response.writeHead(404).end(),
    );
  });

  return new Promise((ready) => server.listen(0, "127.0.0.1", () => ready(server)));
}

const server = await serve();
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch();
const page = await browser.newPage();

const index = await (await fetch(`${origin}/index.json`)).json();
const entries = Object.values(index.entries);
const broken = [];

for (const entry of entries) {
  const view = entry.type === "docs" ? "docs" : "story";
  await page.goto(`${origin}/iframe.html?id=${encodeURIComponent(entry.id)}&viewMode=${view}`);
  try {
    // Storybook settles the body into one of these two, so there is a moment to
    // wait for rather than a duration to guess at.
    await page.waitForFunction(
      () =>
        document.body.classList.contains("sb-show-main") ||
        document.body.classList.contains("sb-show-errordisplay"),
      null,
      { timeout: 30_000 },
    );
    // A page can show its content and then throw; the card replaces it.
    await page.waitForTimeout(150);
    if (await page.evaluate(() => document.body.classList.contains("sb-show-errordisplay"))) {
      const reason = await page.evaluate(
        () => document.querySelector("#error-message")?.textContent ?? "",
      );
      broken.push({ id: entry.id, reason: reason.trim().slice(0, 160) });
    }
  } catch (error) {
    broken.push({ id: entry.id, reason: `never settled: ${error.message.split("\n")[0]}` });
  }
}

await browser.close();
server.close();

const counted = `${entries.length} pages (${entries.filter((e) => e.type === "docs").length} docs)`;
if (broken.length > 0) {
  console.error(`${broken.length} of ${counted} render as an error in Storybook itself:\n`);
  for (const { id, reason } of broken) {
    console.error(`  ${id}\n    ${reason}`);
  }
  process.exit(1);
}
console.log(`${counted} render in Storybook itself.`);
