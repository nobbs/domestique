import { useEffect, useState } from "react";

/** Tracks an element's rendered height. Mirrors `useElementWidth`. */
export function useElementHeight<T extends HTMLElement>() {
  const [element, setElement] = useState<T | null>(null);
  const [height, setHeight] = useState(0);

  useEffect(() => {
    if (!element) {
      return;
    }
    setHeight(element.clientHeight);

    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => setHeight(element.clientHeight));
    observer.observe(element);

    return () => observer.disconnect();
  }, [element]);

  return { ref: setElement, height };
}
