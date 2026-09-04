import { afterEach, describe, expect, it } from "vitest";
import { lockPageZoom } from "./pageZoom";

let undo: (() => void) | undefined;

function lock(): void {
  undo = lockPageZoom();
}

function unlock(): void {
  undo?.();
  undo = undefined;
}

function pinch(name: string): boolean {
  const event = new Event(name, { bubbles: true, cancelable: true });
  document.body.dispatchEvent(event);

  return event.defaultPrevented;
}

afterEach(unlock);

describe("lockPageZoom", () => {
  it.each(["gesturestart", "gesturechange", "gestureend"])(
    "cancels %s so the page does not scale with the pinch",
    (name) => {
      lock();

      expect(pinch(name)).toBe(true);
    },
  );

  it("leaves the gesture alone once undone", () => {
    lock();
    unlock();

    expect(pinch("gesturestart")).toBe(false);
  });
});
