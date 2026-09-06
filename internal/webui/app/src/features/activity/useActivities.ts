/**
 * The rider's own recorded activities, their whole history. The list and the
 * ride page share this one query, so the list's answer is still fresh when a
 * ride is opened from it.
 */

import { useQuery } from "@tanstack/react-query";
import { activitiesQuery } from "../../api/queries";
import type { Activity } from "../../api/types";

export function useActivities(): {
  activities: Activity[];
  isPending: boolean;
  isError: boolean;
} {
  const { data, isPending, isError } = useQuery(activitiesQuery());

  return { activities: data ?? [], isPending, isError };
}
