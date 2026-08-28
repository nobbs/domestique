import { useEffect, useRef, useState } from "react";

/**
 * Reports whether an element has come near the viewport, and stays true once it
 * has.
 *
 * The library's result list uses this to avoid building a map for every row at
 * once: a browser allows only a limited number of live WebGL contexts, so a
 * long library would otherwise start blanking its own maps.
 */
export function useInView<T extends HTMLElement>(rootMargin = "200px") {
  const ref = useRef<T | null>(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element || inView) {
      return;
    }
    // Environments without IntersectionObserver (jsdom) render eagerly rather
    // than never.
    if (typeof IntersectionObserver === "undefined") {
      setInView(true);

      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          // inView latches, so stop observing here rather than waiting for the
          // effect cleanup: scrolling would otherwise keep firing the callback.
          observer.disconnect();
          setInView(true);
        }
      },
      { rootMargin },
    );
    observer.observe(element);

    return () => observer.disconnect();
  }, [inView, rootMargin]);

  return { ref, inView };
}
