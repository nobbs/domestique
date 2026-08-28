/**
 * A deterministic stand-in for the MapLibre canvas, for stories that review
 * chrome and controls rather than rendered cartography.
 *
 * See `MapImplementationContext` in `MapWidget.tsx` for why this is a context
 * seam rather than a module mock: `react-map-gl/maplibre` re-exports via
 * `export *`, which Storybook's automock cannot transform.
 *
 * `useMap()` is untouched — it's the real hook from `react-map-gl/maplibre`,
 * and with no real `<Map>` mounted it reports no current map, same as it does
 * before the real map has loaded. `MapControls` and `MapViewport` already
 * handle that (buttons disabled, effects no-op); a story that specifically
 * needs a live map instance belongs with the six "content" stories that keep
 * the real canvas instead.
 */

import { type ReactNode, useEffect, useRef } from "react";
import { MapImplementationContext, type MapImplementationProps } from "../components/map/MapWidget";

function FakeMap({
  children,
  mapStyle,
  onLoad,
  onIdle,
  style,
  "aria-label": ariaLabel,
}: MapImplementationProps) {
  const loaded = useRef(false);

  // `onIdle`, not just `onLoad`, fires on every run: a story whose controls
  // swap `mapStyle` needs `MapWidget` to see that style reported loaded again,
  // and it is `onIdle` that does that (see the comment on `MapWidget`'s own).
  // biome-ignore lint/correctness/useExhaustiveDependencies: mapStyle is read nowhere in the body; it is the re-run signal, not a value the effect needs.
  useEffect(() => {
    if (!loaded.current) {
      loaded.current = true;
      onLoad?.();
    }
    onIdle?.();
  }, [mapStyle, onLoad, onIdle]);

  return (
    <section
      aria-label={ariaLabel ?? "Map"}
      style={style}
      className="relative h-full w-full bg-[repeating-linear-gradient(45deg,var(--muted),var(--muted)_10px,var(--border)_10px,var(--border)_20px)]"
    >
      {children}
    </section>
  );
}

/** Wraps a story with the deterministic placeholder in place of the real map canvas. */
export function ChromeMap({ children }: { children: ReactNode }) {
  return (
    <MapImplementationContext.Provider value={FakeMap}>
      {children}
    </MapImplementationContext.Provider>
  );
}
