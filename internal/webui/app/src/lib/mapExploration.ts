/**
 * Reading the page the map sits in, or exploring the map itself.
 *
 * A map that fills half the page is also half of everything the wheel and the
 * finger pass over on their way down it, and a map that took either one the
 * moment it was over the canvas would make scrolling past it a thing to be
 * careful about. MapLibre's own answer to that is a cooperative-gesture
 * overlay: it lets the page have the gesture and prints "use Ctrl + scroll to
 * zoom" over the cartography to say so. The instruction is unstyleable,
 * untranslatable, and appears in answer to an ordinary scroll — it interrupts
 * the reader who was not asking for the map to explain how to zoom.
 *
 * So the choice is made once, deliberately, rather than argued about on every
 * gesture. Reading is the resting state: the wheel scrolls the page, a finger
 * anywhere but the route scrolls the page, and the arrow keys move the page.
 * Exploring is entered by a visible control, and until it is left the map has
 * the wheel, the fingers, and — since entering it puts focus on the canvas —
 * the arrow keys, with no modifier to hold down and nothing printed over the
 * ground.
 *
 * What does not change with the mode is the route itself. A drag that begins on
 * the painted line picks a stretch in either mode, because that gesture is
 * about the ride rather than about the view, and `mapSelection` decides it by
 * nearness to the line long before this module has an opinion. Reading mode
 * only has to let such a finger through: everything else it holds back for the
 * page.
 *
 * The map arrives through a small structural interface rather than as
 * MapLibre's `Map`, for the same reason the selection gesture takes one — the
 * four elements and two handlers this switches between are a thing a test can
 * supply without a GPU.
 */

/** A MapLibre handler, as far as turning it off and on again goes. */
interface Handler {
  enable(): void;
  disable(): void;
}

/** As much of a map as choosing between the page and the map needs. */
export interface ExplorableMap {
  /** The outer element, which is where a touch is caught before MapLibre's own
   * listeners on the canvas container can hear it. */
  getContainer(): HTMLElement;
  getCanvasContainer(): HTMLElement;
  getCanvas(): HTMLElement;
  scrollZoom: Handler;
  keyboard: Handler;
}

/**
 * What the canvas leaves to the browser while the page is being read.
 *
 * Naming the page's directions rather than saying `auto` keeps the pinch that
 * would zoom the page as a whole out of a gesture aimed at the map, while
 * leaving the scroll that carries the reader past it exactly as it is
 * everywhere else on the page.
 */
export const PAGE_TOUCH_ACTION = "pan-x pan-y";

/** And what it keeps for itself once exploration has been asked for. */
export const MAP_TOUCH_ACTION = "none";

export interface ExplorationOptions {
  /** Whether the map has been given the gestures, or the page still has them. */
  exploring: boolean;
  /**
   * Whether a finger landing here is drawing along the route.
   *
   * Asked only while the page is being read, and only of a single finger: it is
   * the one touch reading mode hands to the map, because a drag along the line
   * is a question about the ride that the page has no answer to.
   */
  claimsTouch: (point: { clientX: number; clientY: number }) => boolean;
}

/**
 * Puts the map in one of its two modes. Returns the way to take that mode back
 * off it again.
 *
 * Called afresh for each mode rather than mutated in place: which handlers are
 * on, what the canvas leaves to the browser, and whether a finger is being
 * intercepted at all are one decision with one description, and a switch that
 * had to undo the other mode's half of it would be that description twice.
 */
export function mapExploration(map: ExplorableMap, options: ExplorationOptions): () => void {
  const canvasContainer = map.getCanvasContainer();
  const previousTouchAction = canvasContainer.style.touchAction;
  const restore = () => {
    canvasContainer.style.touchAction = previousTouchAction;
  };

  if (options.exploring) {
    map.scrollZoom.enable();
    map.keyboard.enable();
    canvasContainer.style.touchAction = MAP_TOUCH_ACTION;
    // The canvas is where MapLibre listens for the arrow keys, so exploration
    // asked for from the control is exploration the keyboard can carry on with.
    // It is also the only way a reader who cannot use a pointer reaches the
    // gestures this mode exists to grant.
    map.getCanvas().focus();

    return restore;
  }

  map.scrollZoom.disable();
  map.keyboard.disable();
  canvasContainer.style.touchAction = PAGE_TOUCH_ACTION;

  /**
   * Keeps a touch meant for the page away from the map's gesture handlers.
   *
   * `touch-action` alone is not enough. The browser decides what a touch is
   * over the first few pixels of movement, and MapLibre is listening in the
   * meantime: by the time the page starts scrolling the map has already panned
   * under the finger and is about to be told the gesture was cancelled. So the
   * touch is caught on the outer container, one element above the listeners
   * MapLibre puts on the canvas container, and simply not passed on. Nothing is
   * prevented — the page still scrolls from here, which is the whole point —
   * and a gesture MapLibre never hears the start of is one its handlers stay
   * out of for as long as the finger is down.
   */
  const onTouchStart = (event: TouchEvent) => {
    const touch = event.touches.length === 1 ? event.touches[0] : undefined;
    if (touch && options.claimsTouch(touch)) {
      return;
    }
    event.stopPropagation();
  };

  const container = map.getContainer();
  container.addEventListener("touchstart", onTouchStart, { capture: true, passive: true });

  return () => {
    container.removeEventListener("touchstart", onTouchStart, { capture: true });
    restore();
  };
}
