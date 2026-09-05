import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThemeToggle as Toggle } from "./ThemeToggle";

/** A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a `Map` behind the two methods the hook uses is enough. */
function stubStorage(): Map<string, string> {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });

  return entries;
}

// A pick outranks storage for as long as the module lives, so each test takes
// its own module instance — see `theme.test.ts` for the same reset.
async function freshToggle(): Promise<typeof Toggle> {
  vi.resetModules();
  return (await import("./ThemeToggle")).ThemeToggle;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

/** The button, whichever scheme it is currently in — its name changes with the choice. */
function toggle(): HTMLElement {
  return screen.getByRole("button", { name: /^Theme: / });
}

describe("the theme toggle", () => {
  it("names the scheme in force and the one a press would choose", async () => {
    stubStorage();
    const ThemeToggle = await freshToggle();
    render(<ThemeToggle />);

    expect(toggle()).toHaveAccessibleName("Theme: system. Switch to light.");
  });

  it("steps through every scheme and comes back round", async () => {
    const user = userEvent.setup();
    stubStorage();
    const ThemeToggle = await freshToggle();
    render(<ThemeToggle />);

    await user.click(toggle());
    expect(toggle()).toHaveAccessibleName("Theme: light. Switch to dark.");

    await user.click(toggle());
    expect(toggle()).toHaveAccessibleName("Theme: dark. Switch to system.");

    await user.click(toggle());
    expect(toggle()).toHaveAccessibleName("Theme: system. Switch to light.");
  });

  it("remembers the pick for the next visit", async () => {
    const user = userEvent.setup();
    const entries = stubStorage();
    const ThemeToggle = await freshToggle();
    render(<ThemeToggle />);

    await user.click(toggle());

    expect(entries.get("domestique.theme")).toBe("light");
  });

  // The glyph is the only thing a sighted reader has to go on, so it has to
  // follow the choice rather than stand for the control.
  it("draws a different glyph for each scheme", async () => {
    const user = userEvent.setup();
    stubStorage();
    const ThemeToggle = await freshToggle();
    const { container } = render(<ThemeToggle />);
    const glyphOf = () => container.querySelector("svg")?.getAttribute("class");

    const system = glyphOf();
    await user.click(toggle());
    const light = glyphOf();
    await user.click(toggle());

    expect(new Set([system, light, glyphOf()]).size).toBe(3);
  });
});
