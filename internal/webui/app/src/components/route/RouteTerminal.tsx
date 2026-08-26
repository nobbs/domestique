import { IconFlagCheck, IconPlayerPlay } from "@tabler/icons-react";
import { Marker } from "react-map-gl/maplibre";
import type { Position } from "../../api/types";

/** The terminals use distinct pictograms, not two variants of the same dot. */
export function RouteTerminal({
  kind,
  position,
  offset,
  accent,
}: {
  kind: "start" | "finish";
  position: Position;
  offset: number;
  accent: string;
}) {
  return (
    <Marker
      longitude={position[0]}
      latitude={position[1]}
      anchor="center"
      offset={[offset, 0]}
      className={`route-terminal-marker route-terminal-marker--${kind}`}
    >
      {kind === "start" ? (
        <span className="route-terminal route-terminal--start" aria-hidden="true">
          <IconPlayerPlay color="#ffffff" size={18} stroke={3} />
        </span>
      ) : (
        <span className="route-terminal route-terminal--finish" aria-hidden="true">
          <IconFlagCheck color={accent} size={19} stroke={2.5} />
        </span>
      )}
    </Marker>
  );
}
