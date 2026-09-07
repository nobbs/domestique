/**
 * One recorded ride: where it went, and how much of it was uphill. The summary
 * is read from the same activities query the list uses, so arriving from the
 * list costs only the track request; a direct link fetches both.
 */

import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { activityTrackQuery } from "../../api/queries";
import type { Activity, ActivitySeriesName, ActivityTrackState } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { formatAscent, formatDistance, formatDuration, formatTimestamp } from "../../lib/format";
import { buildActivityProfile, type Profile } from "../../lib/profile";
import { ElevationProfile } from "../routes/ElevationProfile";
import { ActivityMap } from "./ActivityMap";
import { RideConditions } from "./RideConditions";
import { RideFigures } from "./RideFigures";
import { SeriesChips, useRideSeries } from "./RideSeries";
import { TrainingLoad } from "./TrainingLoad";
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
  const profile = useMemo(() => buildActivityProfile(coordinates), [coordinates]);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [shown, setShown] = useState<ReadonlySet<ActivitySeriesName>>(() => new Set());
  const { drawn, states } = useRideSeries(id, shown, coordinates, profile);
  const title = ride ? formatTimestamp(ride.startedAt) : "Activity";
  const toggle = useCallback((series: ActivitySeriesName) => {
    setShown((current) => {
      const next = new Set(current);
      if (!next.delete(series)) {
        next.add(series);
      }

      return next;
    });
  }, []);
  // Which sample the shared cursor is on, so a chip can say what its series
  // read there. The profile's samples are evenly spaced across its own stretch.
  const activeIndex = useMemo(
    () => (profile && activeMetres !== null ? sampleIndexAt(profile, activeMetres) : null),
    [profile, activeMetres],
  );

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Link className="text-[var(--ink-2)] text-xs underline" to="/activities">
            Activities
          </Link>
          <h1 className="font-semibold text-2xl tracking-tight">{title}</h1>
          {ride ? <p className="text-[var(--ink-2)] text-sm">{totalsLine(ride)}</p> : null}
        </div>
        {/*
         * Above the map: what the ride came to is what a rider looks for first,
         * and it is there whether or not the ride recorded a position at all.
         */}
        <RideFigures ride={ride} />
        <TrainingLoad metrics={ride?.metrics} />
        {id === null ? (
          <p className="text-[var(--ink-2)] text-sm">{absenceMessage(undefined)}</p>
        ) : track.isPending ? (
          <Skeleton className="h-96 w-full" role="status" aria-label="Loading the recorded track" />
        ) : track.isError || !track.data?.bbox || coordinates.length < 2 ? (
          <p className="text-[var(--ink-2)] text-sm">{absenceMessage(track.data?.state)}</p>
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
            <RideConditions steps={track.data.weather} />
            {profile ? (
              <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
                <ElevationProfile
                  profile={profile}
                  title={title}
                  series={drawn}
                  activeMetres={activeMetres}
                  onActiveChange={setActiveMetres}
                />
                <SeriesChips
                  states={states}
                  drawn={drawn}
                  activeIndex={activeIndex}
                  onToggle={toggle}
                />
              </div>
            ) : null}
          </>
        )}
      </div>
    </PageShell>
  );
}

/**
 * What the ride came to, in one line. Elapsed time is only worth a reader's
 * eye where it says something moving time did not, so a ride that barely
 * stopped shows the one figure rather than two near-identical ones.
 */
function totalsLine(ride: Activity): string {
  const parts = [
    formatDistance(ride.distanceMetres),
    `${formatDuration(ride.movingSeconds)} moving`,
  ];
  if (ride.elapsedSeconds - ride.movingSeconds >= 60) {
    parts.push(`${formatDuration(ride.elapsedSeconds)} elapsed`);
  }
  parts.push(formatAscent(ride.ascentMetres));

  return parts.join(" · ");
}

/**
 * The profile sample nearest one position along the ride.
 *
 * The samples are evenly spaced across the stretch the profile describes, so
 * this is arithmetic rather than a search — and it is the same index the
 * aligned series are laid out on.
 */
function sampleIndexAt(profile: Profile, metres: number): number | null {
  const span = profile.endMetres - profile.startMetres;
  const last = profile.samples.length - 1;
  if (span <= 0 || last < 0) {
    return null;
  }
  const index = Math.round(((metres - profile.startMetres) / span) * last);

  return Math.min(Math.max(index, 0), last);
}

/**
 * Why there is no line to draw, in the rider's own terms: a ride still waiting
 * on its samples is not the same as one that recorded too few to draw. A ride the
 * page holds no state for — not found, or never asked about — keeps the
 * general sentence.
 */
function absenceMessage(state: ActivityTrackState | undefined): string {
  switch (state) {
    case "pending":
      return "The ride's samples have not been read from Wahoo yet.";
    case "empty":
      return "This ride recorded too few positions to draw a line.";
    case "unreadable":
      return "The ride's recorded file could not be read.";
    default:
      return "No recorded track was stored for this ride.";
  }
}
