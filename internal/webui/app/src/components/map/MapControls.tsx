/** Location and zoom controls placed over a MapWidget. */

import { IconCurrentLocation, IconMapPinFilled, IconMinus, IconPlus } from "@tabler/icons-react";
import { useState } from "react";
import { Marker, useMap } from "react-map-gl/maplibre";

const LOCATION_ZOOM = 12;

export function MapControls() {
  const { current: map } = useMap();
  const [location, setLocation] = useState<{ longitude: number; latitude: number } | null>(null);
  const geolocationAvailable = typeof navigator !== "undefined" && "geolocation" in navigator;

  const locate = () => {
    if (!geolocationAvailable) {
      return;
    }

    navigator.geolocation.getCurrentPosition(({ coords }) => {
      setLocation({ longitude: coords.longitude, latitude: coords.latitude });
      map?.flyTo({
        center: [coords.longitude, coords.latitude],
        zoom: Math.max(map.getZoom(), LOCATION_ZOOM),
      });
    });
  };

  return (
    <>
      {location ? (
        <Marker longitude={location.longitude} latitude={location.latitude} anchor="bottom">
          <div className="current-location-marker" role="img" aria-label="Your location">
            <IconMapPinFilled size={18} aria-hidden="true" />
          </div>
        </Marker>
      ) : null}
      <div className="map-controls">
        <button
          className="map-controls__button"
          type="button"
          onClick={locate}
          disabled={!geolocationAvailable}
          aria-label="Find my location"
          title="Find my location"
        >
          <IconCurrentLocation size={16} stroke={2} aria-hidden="true" />
        </button>
        <div className="map-controls__zoom">
          <button type="button" onClick={() => map?.zoomIn()} aria-label="Zoom in" title="Zoom in">
            <IconPlus size={16} stroke={2} aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={() => map?.zoomOut()}
            aria-label="Zoom out"
            title="Zoom out"
          >
            <IconMinus size={16} stroke={2} aria-hidden="true" />
          </button>
        </div>
      </div>
    </>
  );
}
