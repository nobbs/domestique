import { afterEach, describe, expect, it, vi } from "vitest";
import type { Position } from "../api/types";
import { routeSelection, type SelectableMap } from "./mapSelection";
import { buildProfile, type DistanceWindow, type Profile } from "./profile";
import { MIN_WINDOW_METRES } from "./selection";

/**
 * A route drawn due east along one line of latitude, so a pixel across the fake
 * map is a known distance along the ride and every expectation below can be
 * written as "this far in".
 */
function eastward(): Position[] {
  return Array.from({ length: 81 }, (_, index): Position => [8 + index * 0.0005, 49, 100]);
}

function eastwardProfile(): Profile {
  const profile = buildProfile(eastward());
  if (!profile) {
    throw new Error("the fixture route has no profile");
  }

  return profile;
}

const PROFILE = eastwardProfile();

/** Pixels per degree, which is all the camera this needs. */
const SCALE = 20_000;
/** How wide the whole route is drawn, at that scale. */
const ROUTE_PIXELS = 80 * 0.0005 * SCALE;
const ROUTE_METRES = PROFILE.totalDistanceMetres;

/** Where along the route a pointer this far across the map lands. */
function metresAt(pixels: number): number {
  return (pixels / ROUTE_PIXELS) * ROUTE_METRES;
}

/**
 * Within one sample of the profile.
 *
 * The gesture answers in samples — it can only settle on ground the profile
 * describes — so an expectation written in pixels is right to within the
 * spacing between them, and pretending otherwise would be a test of arithmetic
 * this module does not do.
 */
const SAMPLE_METRES = ROUTE_METRES / (PROFILE.samples.length - 1);

function expectMetres(actual: number, expected: number) {
  expect(Math.abs(actual - expected)).toBeLessThanOrEqual(SAMPLE_METRES);
}

interface FakeMap extends SelectableMap {
  container: HTMLElement;
  panEnabled: () => boolean;
}

function fakeMap(panEnabled = true): FakeMap {
  const container = document.createElement("div");
  document.body.append(container);
  let pan = panEnabled;

  return {
    container,
    panEnabled: () => pan,
    getCanvasContainer: () => container,
    project: ([lng, lat]) => ({ x: (lng - 8) * SCALE, y: (49 - lat) * SCALE }),
    unproject: ([x, y]) => ({ lng: 8 + x / SCALE, lat: 49 - y / SCALE }),
    dragPan: {
      enable: () => {
        pan = true;
      },
      disable: () => {
        pan = false;
      },
      isEnabled: () => pan,
    },
  };
}

interface PointerOptions {
  pointerId?: number;
  isPrimary?: boolean;
  pointerType?: string;
  button?: number;
}

function pointer(
  type: string,
  x: number,
  y: number,
  { pointerId = 1, isPrimary = true, pointerType = "mouse", button = 0 }: PointerOptions = {},
) {
  return new PointerEvent(type, {
    clientX: x,
    clientY: y,
    pointerId,
    isPrimary,
    pointerType,
    button,
    bubbles: true,
    cancelable: true,
  });
}

/**
 * A touch landing at these points.
 *
 * jsdom has no touch input of its own, so the event carries only what the
 * gesture reads of one: where the fingers are, and whether the answer to it can
 * still be given.
 */
function touchStart(...points: Array<[number, number]>): Event {
  const event = new Event("touchstart", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "touches", {
    value: points.map(([x, y]) => ({ clientX: x, clientY: y })),
  });

  return event;
}

/** A gesture, with the map and the answers it gave, ready to assert against. */
function selecting(panEnabled = true) {
  const map = fakeMap(panEnabled);
  const onSelect = vi.fn<(window: DistanceWindow) => void>();
  const onPending = vi.fn<(window: DistanceWindow | null) => void>();
  const dispose = routeSelection(map, { profile: PROFILE, onPending, onSelect });
  disposers.push(dispose);

  return {
    map,
    onSelect,
    onPending,
    dispose,
    down: (x: number, y: number, options?: PointerOptions) =>
      map.container.dispatchEvent(pointer("pointerdown", x, y, options)),
    move: (x: number, y: number, options?: PointerOptions) =>
      window.dispatchEvent(pointer("pointermove", x, y, options)),
    up: (x: number, y: number, options?: PointerOptions) =>
      window.dispatchEvent(pointer("pointerup", x, y, options)),
    cancel: (x: number, y: number, options?: PointerOptions) =>
      window.dispatchEvent(pointer("pointercancel", x, y, options)),
    touch: (...points: Array<[number, number]>) => {
      const event = touchStart(...points);
      map.container.dispatchEvent(event);

      return event;
    },
  };
}

const disposers: Array<() => void> = [];

afterEach(() => {
  while (disposers.length > 0) {
    disposers.pop()?.();
  }
  document.body.replaceChildren();
});

describe("selecting a stretch by dragging the route", () => {
  it("picks the ground the drag covered", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(300, 0);
    gesture.up(300, 0);

    expect(gesture.onSelect).toHaveBeenCalledTimes(1);
    const chosen = gesture.onSelect.mock.calls[0]?.[0];
    expectMetres(chosen?.startMetres ?? 0, metresAt(100));
    expectMetres(chosen?.endMetres ?? 0, metresAt(300));
  });

  it("reads a drag back along the route the same as one along it", () => {
    const gesture = selecting();
    gesture.down(300, 0);
    gesture.move(100, 0);
    gesture.up(100, 0);

    const chosen = gesture.onSelect.mock.calls[0]?.[0];
    expectMetres(chosen?.startMetres ?? 0, metresAt(100));
    expectMetres(chosen?.endMetres ?? 0, metresAt(300));
  });

  it("shows the stretch under the hand while it is still being drawn", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(200, 0);

    const pending = gesture.onPending.mock.calls.at(-1)?.[0];
    expectMetres(pending?.endMetres ?? 0, metresAt(200));
    expect(gesture.onSelect).not.toHaveBeenCalled();

    gesture.up(200, 0);
    // Cleared before the window it became arrives, so nothing is lit twice.
    expect(gesture.onPending).toHaveBeenLastCalledWith(null);
  });

  it("says nothing again about a stretch that has not changed", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(300, 0);
    const reports = gesture.onPending.mock.calls.length;

    gesture.move(300, 0);
    gesture.move(300, 0);
    expect(gesture.onPending).toHaveBeenCalledTimes(reports);
  });

  it("stands the map's pan down for the drag, and gives it straight back", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    expect(gesture.map.panEnabled()).toBe(false);

    gesture.move(300, 0);
    gesture.up(300, 0);
    expect(gesture.map.panEnabled()).toBe(true);
  });

  it("leaves a drag that began away from the route to the map", () => {
    const gesture = selecting();
    gesture.down(100, 60);
    expect(gesture.map.panEnabled()).toBe(true);

    gesture.move(300, 60);
    gesture.up(300, 60);
    expect(gesture.onSelect).not.toHaveBeenCalled();
    expect(gesture.onPending).not.toHaveBeenCalled();
  });

  it("takes a hand that barely moved as pointing, not as a selection", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(104, 0);
    gesture.up(104, 0);

    expect(gesture.onSelect).not.toHaveBeenCalled();
    expect(gesture.map.panEnabled()).toBe(true);
  });

  it("settles on the last ground the pointer was near, not on wherever it ended", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(300, 0);
    // Off the line entirely: the hand has left the road, not asked for the far
    // side of it.
    gesture.move(500, 400);
    gesture.up(500, 400);

    const chosen = gesture.onSelect.mock.calls[0]?.[0];
    expectMetres(chosen?.endMetres ?? 0, metresAt(300));
  });

  it("grows a stretch too short to plot rather than refusing it", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(112, 0);
    gesture.up(112, 0);

    const chosen = gesture.onSelect.mock.calls[0]?.[0];
    expect((chosen?.endMetres ?? 0) - (chosen?.startMetres ?? 0)).toBeCloseTo(MIN_WINDOW_METRES, 6);
  });

  it("keeps a grown stretch inside the route it was drawn on", () => {
    const gesture = selecting();
    gesture.down(0, 0);
    gesture.move(10, 0);
    gesture.up(10, 0);

    expect(gesture.onSelect).toHaveBeenCalledWith({
      startMetres: 0,
      endMetres: MIN_WINDOW_METRES,
    });
  });

  it("picks by touch exactly as it does by mouse", () => {
    const gesture = selecting();
    const touch = { pointerType: "touch", pointerId: 7 };
    gesture.down(100, 0, touch);
    gesture.move(300, 0, touch);
    gesture.up(300, 0, touch);

    const chosen = gesture.onSelect.mock.calls[0]?.[0];
    expectMetres(chosen?.startMetres ?? 0, metresAt(100));
    expectMetres(chosen?.endMetres ?? 0, metresAt(300));
  });
});

describe("abandoning a stretch being drawn", () => {
  it("commits nothing when the gesture is cancelled out from under it", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(300, 0);
    gesture.cancel(300, 0);

    expect(gesture.onSelect).not.toHaveBeenCalled();
    expect(gesture.onPending).toHaveBeenLastCalledWith(null);
    expect(gesture.map.panEnabled()).toBe(true);
  });

  it("abandons the stretch on Escape, and leaves the view it was drawn on alone", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.move(300, 0);

    const pressed = new KeyboardEvent("keydown", {
      key: "Escape",
      bubbles: true,
      cancelable: true,
    });
    document.dispatchEvent(pressed);

    expect(pressed.defaultPrevented).toBe(true);
    expect(gesture.onSelect).not.toHaveBeenCalled();
    expect(gesture.onPending).toHaveBeenLastCalledWith(null);
    expect(gesture.map.panEnabled()).toBe(true);
  });

  it("leaves Escape alone while nothing is being drawn", () => {
    selecting();
    const pressed = new KeyboardEvent("keydown", {
      key: "Escape",
      bubbles: true,
      cancelable: true,
    });
    document.dispatchEvent(pressed);

    expect(pressed.defaultPrevented).toBe(false);
  });

  it("hands the map back to a second finger", () => {
    const gesture = selecting();
    gesture.down(100, 0, { pointerType: "touch", pointerId: 1 });
    gesture.move(200, 0, { pointerType: "touch", pointerId: 1 });
    gesture.down(300, 0, { pointerType: "touch", pointerId: 2, isPrimary: false });

    expect(gesture.map.panEnabled()).toBe(true);

    gesture.up(200, 0, { pointerType: "touch", pointerId: 1 });
    expect(gesture.onSelect).not.toHaveBeenCalled();
  });

  it("draws nothing from a press of the button that opens a menu", () => {
    const gesture = selecting();
    gesture.down(100, 0, { button: 2 });
    gesture.move(300, 0);
    gesture.up(300, 0);

    expect(gesture.onSelect).not.toHaveBeenCalled();
    expect(gesture.map.panEnabled()).toBe(true);
  });
});

describe("taking the gesture off the map", () => {
  it("stops listening, and leaves the pan as it found it", () => {
    const gesture = selecting();
    gesture.down(100, 0);
    gesture.dispose();
    expect(gesture.map.panEnabled()).toBe(true);

    gesture.move(300, 0);
    gesture.up(300, 0);
    expect(gesture.onSelect).not.toHaveBeenCalled();
  });

  it("does not switch a pan back on that was already off", () => {
    const gesture = selecting(false);
    gesture.down(100, 0);
    gesture.move(300, 0);
    gesture.up(300, 0);

    expect(gesture.map.panEnabled()).toBe(false);
    expect(gesture.onSelect).toHaveBeenCalledTimes(1);
  });
});

/**
 * What the page does with the same finger.
 *
 * A touch is offered to the browser before it is offered to anything on the
 * page, and a map that fills most of a phone has to keep being somewhere the
 * page can be scrolled from. So the only question these ask is which of the two
 * a finger belongs to, decided where it lands.
 */
describe("a finger, between the route and the page", () => {
  it("claims one that lands on the route", () => {
    const gesture = selecting();

    expect(gesture.touch([100, 0]).defaultPrevented).toBe(true);
  });

  it("leaves one that lands away from the route to the page", () => {
    const gesture = selecting();

    expect(gesture.touch([100, 200]).defaultPrevented).toBe(false);
  });

  // Two fingers are an exploration of the map, and cooperative gestures are
  // already the answer to what they mean.
  it("leaves a second finger alone", () => {
    const gesture = selecting();

    expect(gesture.touch([100, 0], [300, 0]).defaultPrevented).toBe(false);
  });

  it("stops claiming anything once it is disposed", () => {
    const gesture = selecting();
    gesture.dispose();

    expect(gesture.touch([100, 0]).defaultPrevented).toBe(false);
  });

  // A fingertip covers more of the screen than it can aim with, so it is given
  // more of the line to aim at than a cursor is.
  it("reaches further for a fingertip than for a cursor", () => {
    const byFinger = selecting();
    byFinger.down(100, 28, { pointerType: "touch", pointerId: 3 });
    byFinger.move(300, 28, { pointerType: "touch", pointerId: 3 });
    byFinger.up(300, 28, { pointerType: "touch", pointerId: 3 });

    expect(byFinger.onSelect).toHaveBeenCalledTimes(1);

    const byCursor = selecting();
    byCursor.down(100, 28);
    byCursor.move(300, 28);
    byCursor.up(300, 28);

    expect(byCursor.onSelect).not.toHaveBeenCalled();
    expect(byCursor.map.panEnabled()).toBe(true);
  });

  it("claims the wider reach for the page's finger too", () => {
    const gesture = selecting();

    expect(gesture.touch([100, 28]).defaultPrevented).toBe(true);
  });
});
