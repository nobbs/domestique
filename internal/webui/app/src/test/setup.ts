import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect } from "vitest";
import { refusingFetch } from "./network";

/**
 * A `matchMedia` for jsdom, which has none.
 *
 * Components ask the platform two things — the viewport width and whether less
 * movement was asked for — and a component that asks throws in this environment
 * rather than falling back. The stub answers "no" to everything, so a test that
 * says nothing about the viewport gets the wide layout and full motion, and a
 * test that cares stubs it again with the answers it wants.
 */
beforeEach(() => {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
});

/**
 * And a `scrollIntoView`, which jsdom has none of either.
 *
 * It lays nothing out, so it implements none of the scrolling API. A component
 * that brings itself into view is asking the platform to do something with a
 * geometry this environment does not have; doing nothing is the right answer.
 */
Element.prototype.scrollIntoView = () => {};

/** Every request the suite's own `fetch` refused during the current test. */
const requested: string[] = [];

// Assigned rather than stubbed through Vitest: a file that ends its own test
// with `vi.unstubAllGlobals` would otherwise put the platform's `fetch` back
// for the unmount that follows, and a request made there would go out unseen.
beforeEach(() => {
  requested.length = 0;
  globalThis.fetch = refusingFetch(requested);
});

afterEach(() => {
  const made = [...requested];
  requested.length = 0;

  expect(made).toEqual([]);
});

// Testing Library only registers its own cleanup when Vitest globals are on.
// This suite imports its helpers explicitly, so unmount between tests here or
// each render leaks into the next one's queries.
afterEach(cleanup);
