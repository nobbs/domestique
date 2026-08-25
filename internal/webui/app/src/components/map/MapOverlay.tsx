/** Renders ordinary HTML above a MapWidget without making it part of the canvas. */

import { type ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useMap } from "react-map-gl/maplibre";

export function MapOverlay({ children }: { children: ReactNode }) {
  const { current: map } = useMap();
  const [container, setContainer] = useState<HTMLElement | null>(null);

  useEffect(() => {
    setContainer(map?.getContainer().parentElement ?? null);
  }, [map]);

  return container ? createPortal(children, container) : null;
}
