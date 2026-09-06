/**
 * One recorded ride: where it went, and how much of it was uphill. The summary
 * is read from the same activities query the list uses, so arriving from the
 * list costs only the track request; a direct link fetches both.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { activityTrackQuery } from "../../api/queries";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { formatAscent, formatDistance, formatMovingTime, formatTimestamp } from "../../lib/format";
import { buildProfile } from "../../lib/profile";
import { ElevationProfile } from "../routes/ElevationProfile";
import { ActivityMap } from "./ActivityMap";
import { useActivities } from "./useActivities";

export function ActivityPage() {
  const { activityId } = useParams();
  // Only a run of digits names an activity; anything else (a decimal, "NaN",
  // stray text) must never reach the track endpoint as a path segment.
  const id = activityId && /^\d+$/.test(activityId) ? Number(activityId) : null;
  const { activities } = useActivities();
  const ride = activities.find((activity) => activity.id === id);
  const track = useQuery({ ...activityTrackQuery(id ?? 0), enabled: id !== null });
  const coordinates = useMemo(() => track.data?.coordinates ?? [], [track.data]);
  // Only where the track carried altitudes: a position without one reads as
  // sea level, which would draw a flat profile rather than none.
  const profile = useMemo(
    () => (coordinates[0]?.length === 3 ? buildProfile(coordinates) : null),
    [coordinates],
  );
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const title = ride ? formatTimestamp(ride.startedAt) : "Activity";

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Link className="text-[var(--ink-2)] text-xs underline" to="/activities">
            Activities
          </Link>
          <h1 className="font-semibold text-2xl tracking-tight">{title}</h1>
          {ride ? (
            <p className="text-[var(--ink-2)] text-sm">
              {[
                formatDistance(ride.distanceMetres),
                formatMovingTime(ride.movingSeconds),
                formatAscent(ride.ascentMetres),
              ].join(" · ")}
            </p>
          ) : null}
        </div>
        {id === null ? (
          <p className="text-[var(--ink-2)] text-sm">No recorded track was stored for this ride.</p>
        ) : track.isPending ? (
          <Skeleton className="h-96 w-full" role="status" aria-label="Loading the recorded track" />
        ) : track.isError || !track.data || coordinates.length < 2 ? (
          <p className="text-[var(--ink-2)] text-sm">No recorded track was stored for this ride.</p>
        ) : (
          <>
            <div className="h-96 overflow-hidden rounded-xl ring-1 ring-black/5">
              <ActivityMap
                coordinates={coordinates}
                bounds={track.data.bbox}
                profile={profile}
                activeMetres={activeMetres}
                onActiveChange={setActiveMetres}
              />
            </div>
            {profile ? (
              <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
                <ElevationProfile
                  profile={profile}
                  title={title}
                  activeMetres={activeMetres}
                  onActiveChange={setActiveMetres}
                />
              </div>
            ) : null}
          </>
        )}
      </div>
    </PageShell>
  );
}
