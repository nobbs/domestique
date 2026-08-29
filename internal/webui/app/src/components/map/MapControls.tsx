/**
 * Location and zoom controls placed over a MapWidget.
 *
 * Anything else the corner should hold comes in as children and stacks under
 * the zoom pair — the basemap chooser does. It arrives that way rather than as
 * props because what a reader may choose as the ground is the map's business
 * and not these two controls'.
 *
 * The zoom pair is hidden from a coarse pointer, which has pinching instead;
 * the rule for it is `.map-zoom` in `index.css`.
 */

import { IconCurrentLocation, IconMinus, IconPlus } from "@tabler/icons-react";
import { type ReactNode, useState } from "react";
import { Marker, useMap } from "react-map-gl/maplibre";
import { ButtonGroup } from "@/components/ui/button-group";
import { Button } from "../Button";
import { LocationPin } from "./LocationPin";

const LOCATION_ZOOM = 12;

export function MapControls({ children }: { children?: ReactNode }) {
  const { current: map } = useMap();
  const [location, setLocation] = useState<{ longitude: number; latitude: number } | null>(null);
  const geolocationAvailable = typeof navigator !== "undefined" && "geolocation" in navigator;

  const locate = () => {
    if (!geolocationAvailable || !map) {
      return;
    }

    navigator.geolocation.getCurrentPosition(({ coords }) => {
      setLocation({ longitude: coords.longitude, latitude: coords.latitude });
      map.flyTo({
        center: [coords.longitude, coords.latitude],
        zoom: Math.max(map.getZoom(), LOCATION_ZOOM),
      });
    });
  };

  return (
    <>
      {location ? (
        <Marker longitude={location.longitude} latitude={location.latitude} anchor="bottom">
          <LocationPin />
        </Marker>
      ) : null}
      {/* Only where the corner is: the controls themselves are the application's own. */}
      <div className="map-controls">
        <Button
          variant="panel"
          icon={<IconCurrentLocation stroke={2} />}
          onClick={locate}
          disabled={!geolocationAvailable || !map}
          aria-label="Find my location"
          title="Find my location"
        />
        {/*
         * The frame belongs to the group rather than to each button: one edge,
         * one radius, one shadow, and a rule between the buttons instead of
         * around them. It is a ring rather than a border because a border
         * would be laid out as well as drawn, and these buttons are 32 pixels
         * wide including their own edges — a group adding two more would stand
         * two wider than the single button above it.
         */}
        <ButtonGroup
          orientation="vertical"
          className="map-zoom divide-y divide-[var(--rule)] rounded-lg bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-[var(--rule)] ring-inset [&>*:not(:first-child)]:rounded-t-none [&>*:not(:last-child)]:rounded-b-none"
        >
          <Button
            variant="ghost"
            icon={<IconPlus stroke={2} />}
            onClick={() => map?.zoomIn()}
            disabled={!map}
            aria-label="Zoom in"
            title="Zoom in"
          />
          <Button
            variant="ghost"
            icon={<IconMinus stroke={2} />}
            onClick={() => map?.zoomOut()}
            disabled={!map}
            aria-label="Zoom out"
            title="Zoom out"
          />
        </ButtonGroup>
        {children}
      </div>
    </>
  );
}
