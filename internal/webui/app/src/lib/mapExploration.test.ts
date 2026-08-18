import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type ExplorableMap,
  MAP_TOUCH_ACTION,
  mapExploration,
  PAGE_TOUCH_ACTION,
} from "./mapExploration";

/**
 * A map, its elements nested as MapLibre nests them.
 *
 * MapLibre listens for gestures on the canvas container, so `heard` stands in
 * for every handler it has there: a touch that reaches it is a touch the map
 * would have acted on. The two handlers are the ones this module switches, and
 * the canvas is made focusable because that is what MapLibre does to it and
 * what makes the arrow keys reach the map at all.
 */
interface FakeMap extends ExplorableMap {
  container: HTMLElement;
  canvas: HTMLElement;
  heard: ReturnType<typeof vi.fn>;
  scrollZoomEnabled: () => boolean;
  keyboardEnabled: () => boolean;
}

function fakeMap(): FakeMap {
  const container = document.createElement("div");
  const canvasContainer = document.createElement("div");
  const canvas = document.createElement("canvas");
  canvas.tabIndex = 0;
  canvasContainer.append(canvas);
  container.append(canvasContainer);
  document.body.append(container);

  const heard = vi.fn();
  canvasContainer.addEventListener("touchstart", heard);

  let scrollZoom = true;
  let keyboard = true;

  return {
    container,
    canvas,
    heard,
    scrollZoomEnabled: () => scrollZoom,
    keyboardEnabled: () => keyboard,
    getContainer: () => container,
    getCanvasContainer: () => canvasContainer,
    getCanvas: () => canvas,
    scrollZoom: {
      enable: () => {
        scrollZoom = true;
      },
      disable: () => {
        scrollZoom = false;
      },
    },
    keyboard: {
      enable: () => {
        keyboard = true;
      },
      disable: () => {
        keyboard = false;
      },
    },
  };
}

/**
 * Fingers landing on the map.
 *
 * jsdom has no touch input of its own, so the event carries only what is read
 * of one: where the fingers are, and whether the page can still have it. It is
 * dispatched on the canvas, which is where a real one lands, so the capture
 * phase runs through every element between there and the page.
 */
function touch(map: FakeMap, ...points: Array<[number, number]>): Event {
  const event = new Event("touchstart", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "touches", {
    value: points.map(([x, y]) => ({ clientX: x, clientY: y })),
  });
  map.canvas.dispatchEvent(event);

  return event;
}

const disposers: Array<() => void> = [];

/** The route runs along the top of this map, and nothing else is on it. */
const onTheRoute = (point: { clientY: number }) => point.clientY === 0;

function mode(exploring: boolean, claimsTouch = onTheRoute) {
  const map = fakeMap();
  disposers.push(mapExploration(map, { exploring, claimsTouch }));

  return map;
}

afterEach(() => {
  while (disposers.length > 0) {
    disposers.pop()?.();
  }
  document.body.replaceChildren();
});

describe("reading the page the map sits in", () => {
  it("leaves the wheel and the trackpad to the page", () => {
    expect(mode(false).scrollZoomEnabled()).toBe(false);
  });

  it("leaves the arrow keys to the page", () => {
    expect(mode(false).keyboardEnabled()).toBe(false);
  });

  it("leaves the page's own scrolling to the browser", () => {
    expect(mode(false).getCanvasContainer().style.touchAction).toBe(PAGE_TOUCH_ACTION);
  });

  it("takes no focus from wherever the reader was", () => {
    const map = mode(false);

    expect(document.activeElement).not.toBe(map.canvas);
  });

  it("keeps a finger that lands away from the route from ever reaching the map", () => {
    const map = mode(false);
    const finger = touch(map, [100, 200]);

    expect(map.heard).not.toHaveBeenCalled();
    // Not prevented, only withheld: this is the finger the page scrolls with.
    expect(finger.defaultPrevented).toBe(false);
  });

  it("hands a finger drawn along the route to the map", () => {
    const map = mode(false);
    touch(map, [100, 0]);

    expect(map.heard).toHaveBeenCalledTimes(1);
  });

  it("keeps a second finger for the page as well", () => {
    const map = mode(false);
    const fingers = touch(map, [100, 0], [300, 0]);

    expect(map.heard).not.toHaveBeenCalled();
    expect(fingers.defaultPrevented).toBe(false);
  });

  it("asks nothing of the route where no stretch can be picked", () => {
    const claimsTouch = vi.fn(() => false);
    const map = fakeMap();
    disposers.push(mapExploration(map, { exploring: false, claimsTouch }));
    touch(map, [100, 0]);

    expect(claimsTouch).toHaveBeenCalledTimes(1);
    expect(map.heard).not.toHaveBeenCalled();
  });
});

describe("exploring the map", () => {
  it("zooms to the wheel with no modifier to hold", () => {
    expect(mode(true).scrollZoomEnabled()).toBe(true);
  });

  it("answers the arrow keys, and puts the focus where they are heard", () => {
    const map = mode(true);

    expect(map.keyboardEnabled()).toBe(true);
    expect(document.activeElement).toBe(map.canvas);
  });

  it("keeps every finger for the map", () => {
    const map = mode(true);

    expect(map.getCanvasContainer().style.touchAction).toBe(MAP_TOUCH_ACTION);
    touch(map, [100, 200]);
    touch(map, [100, 0], [300, 0]);
    expect(map.heard).toHaveBeenCalledTimes(2);
  });
});

describe("changing the reader's mind", () => {
  it("gives the fingers back to the map on the way in", () => {
    const map = fakeMap();
    const reading = mapExploration(map, { exploring: false, claimsTouch: onTheRoute });
    reading();
    disposers.push(mapExploration(map, { exploring: true, claimsTouch: onTheRoute }));

    touch(map, [100, 200]);
    expect(map.heard).toHaveBeenCalledTimes(1);
    expect(map.getCanvasContainer().style.touchAction).toBe(MAP_TOUCH_ACTION);
  });

  it("takes them back off it on the way out", () => {
    const map = fakeMap();
    const exploring = mapExploration(map, { exploring: true, claimsTouch: onTheRoute });
    exploring();
    disposers.push(mapExploration(map, { exploring: false, claimsTouch: onTheRoute }));

    touch(map, [100, 200]);
    expect(map.heard).not.toHaveBeenCalled();
    expect(map.getCanvasContainer().style.touchAction).toBe(PAGE_TOUCH_ACTION);
  });

  it("leaves the canvas as it found it once the mode is taken off", () => {
    const map = fakeMap();
    map.getCanvasContainer().style.touchAction = "auto";
    mapExploration(map, { exploring: false, claimsTouch: onTheRoute })();

    expect(map.getCanvasContainer().style.touchAction).toBe("auto");
    touch(map, [100, 200]);
    expect(map.heard).toHaveBeenCalledTimes(1);
  });
});
