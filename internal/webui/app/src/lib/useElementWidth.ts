import { useEffect, useRef, useState } from "react";

/**
 * Tracks an element's rendered width.
 *
 * The elevation profile draws in real pixels rather than stretching a fixed
 * viewBox, so its strokes and labels keep their proportions at any card width.
 */
export function useElementWidth<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const element = ref.current;
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
  }, []);

  return { ref, width };
}
