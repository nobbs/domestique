/**
 * The accessibility rules the jsdom suite holds every rendered component to.
 *
 * axe is run against the DOM a component test already built, so a component
 * gains this check by being rendered rather than by anybody remembering to
 * assert it. What it catches is the mechanical half — a control with no
 * accessible name, a label pointing at nothing, a list whose items are not in a
 * list, an ARIA attribute the role does not allow. What it cannot catch is the
 * half this repository writes prose about: whether the name is the right name,
 * whether the focus order matches the reading order, whether a colour is doing
 * work that words should. A green run here is a floor, not a verdict.
 *
 * Colour-contrast rules are excluded because they are unanswerable in jsdom: no
 * stylesheet is loaded — Vitest is configured with `css: false` — so every
 * element would be judged as black on transparent. Contrast is a question for
 * the browser suite, which loads the real stylesheet.
 */

import axe, { type Result, type RunOptions } from "axe-core";
import { expect } from "vitest";

/**
 * Rules that need a whole document to be true of a fragment.
 *
 * A component test renders one component into a bare container, so the page-level
 * rules — landmarks, a top-level heading, a document language — are asking about
 * a page that was never built. The browser suite asserts them against the real
 * page, where the question means something.
 */
const PAGE_RULES = [
  "region",
  "landmark-one-main",
  "page-has-heading-one",
  "html-has-lang",
  "bypass",
];

const OPTIONS: RunOptions = {
  rules: Object.fromEntries(
    [...PAGE_RULES, "color-contrast"].map((rule) => [rule, { enabled: false }]),
  ),
};

/** One violation, as a line somebody reading a failure can act on. */
function describe(violation: Result): string {
  const where = violation.nodes.map((node) => node.target.join(" ")).join(", ");

  return `${violation.id} (${violation.impact}): ${violation.help} — at ${where}`;
}

/**
 * Fails when axe finds anything wrong with the given container.
 *
 * Takes the element rather than reaching for `document.body`, so a suite that
 * renders more than one thing can ask about one of them.
 */
export async function expectNoAxeViolations(container: Element): Promise<void> {
  const { violations } = await axe.run(container, OPTIONS);

  expect(violations.map(describe), "axe found no accessibility violations").toEqual([]);
}
