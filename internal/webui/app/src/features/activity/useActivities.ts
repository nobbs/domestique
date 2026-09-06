/**
 * The rider's own recorded activities, over the same window the Volume page
 * reads. The list and the ride page share this one query, so the list's answer
 * is still fresh when a ride is opened from it.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { activitiesQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity } from "../../api/types";
import { windowStart } from "../../lib/volume";

// Only while the config is unavailable: once it answers, its zone is the one
// used, whether or not it happens to match the browser's own.
const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

export function useActivities(): {
  activities: Activity[];
  isPending: boolean;
  isError: boolean;
} {
  const config = useQuery(webUIConfigQuery());
  const zone = config.data?.timezone || browserZone();
  const from = useMemo(() => windowStart(zone), [zone]);
  const { data, isPending, isError } = useQuery(activitiesQuery(from));

  return { activities: data ?? [], isPending, isError };
}
