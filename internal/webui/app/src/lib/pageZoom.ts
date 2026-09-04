/**
 * Holding the document at one scale on a touch screen.
 *
 * The application is a fixed frame — a menu bar, a map that fills what is left,
 * panels pinned to its corners against the safe-area insets. Scaling that frame
 * does not reveal anything: it pushes the pinned panels off the screen and
 * leaves the reader panning a viewport around a layout that was already the
 * size of the display. The map has its own zoom, and it is the thing a pinch
 * over the map is asking to scale.
 *
 * Two mechanisms, because no single one covers both engines. `touch-action` in
 * `index.css` is what Chrome and Android WebView honour, and it is also what
 * suppresses double-tap zoom everywhere. iOS Safari has ignored both
 * `user-scalable=no` and `touch-action`'s pinch clause for the page since
 * iOS 10, and announces a pinch through the non-standard `gesture*` events
 * instead; cancelling those is the only thing that holds the page still there.
 *
 * MapLibre reads raw touch events and never these, so cancelling them costs the
 * map nothing: pinching the canvas still zooms the camera, it just no longer
 * also zooms the document underneath it.
 */

const GESTURE_EVENTS = ["gesturestart", "gesturechange", "gestureend"];

/** Cancels the browser's own pinch zoom for as long as the returned undo is uncalled. */
export function lockPageZoom(target: EventTarget = document): () => void {
  const cancel = (event: Event) => event.preventDefault();
  for (const name of GESTURE_EVENTS) {
    // Non-passive, or the browser is free to ignore the `preventDefault`.
    target.addEventListener(name, cancel, { passive: false });
  }

  return () => {
    for (const name of GESTURE_EVENTS) {
      target.removeEventListener(name, cancel);
    }
  };
}
