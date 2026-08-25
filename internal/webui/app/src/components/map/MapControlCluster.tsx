/** Places optional furniture in MapLibre's unobstructed lower-right corner. */

import { type ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useMap } from "react-map-gl/maplibre";

const CLUSTER_SELECTOR = ".maplibregl-ctrl-bottom-right";

export function MapControlCluster({ children }: { children: ReactNode }) {
  const { current: map } = useMap();
  const [container, setContainer] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const mapContainer = map?.getContainer();
    setContainer(
      mapContainer?.querySelector(CLUSTER_SELECTOR) ?? mapContainer?.parentElement ?? null,
    );
  }, [map]);

  return container ? createPortal(children, container) : null;
}
