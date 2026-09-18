/**
 * What a waypoint's coordinate is called, where a geocoder is configured.
 * Asked per rounded coordinate so two waypoints in one place share a request,
 * and answered with an empty string wherever there is no name to show.
 */

import { useQuery } from "@tanstack/react-query";
import { getReversePlaceQueryOptions } from "../../api/generated";
import { webUIConfigQuery } from "../../api/queries";
import type { Place } from "../../api/types";

/** About eleven metres: finer than two waypoints anyone would call one place. */
const PRECISION = 4;

const round = (value: number) => Number(value.toFixed(PRECISION));

export function usePlaceName(latitude: number, longitude: number): string {
  const config = useQuery(webUIConfigQuery());
  const params = { latitude: round(latitude), longitude: round(longitude) };
  const place = useQuery({
    ...getReversePlaceQueryOptions(params, {
      query: { select: (response) => (response.data as Place).name ?? "" },
    }),
    enabled: config.data?.placeNames === true,
    // A place does not move, and the answer is already held by the service.
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  });

  return place.data ?? "";
}
