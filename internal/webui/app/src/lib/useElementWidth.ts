import { useEffect, useState } from "react";

/**
 * Tracks an element's rendered width.
 *
 * The elevation profile draws in real pixels rather than stretching a fixed
 * viewBox, so its strokes and labels keep their proportions at any card width.
 *
 * `ref` is a callback ref rather than an object one, and the element it
 * captures is state rather than a mutable box, so measuring starts whenever
 * the element actually arrives. A caller that returns early before mounting it
 * — a chart that draws nothing until its data has loaded — would otherwise
 * measure once against nothing, never hear that the element appeared, and
 * spend the rest of its life drawing at whatever a width of zero falls back
 * to.
 */
export function useElementWidth<T extends HTMLElement>() {
  const [element, setElement] = useState<T | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    if (!element) {
      return;
    }
    setWidth(element.clientWidth);

    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => setWidth(element.clientWidth));
    observer.observe(element);

    return () => observer.disconnect();
  }, [element]);

  return { ref: setElement, width };
}
