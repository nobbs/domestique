/**
 * One recorded ride: the four figures that decide it set large beside the map,
 * then the terrain with the weather laid under it, the effort, and the ride by
 * the kilometre. The summary is read from the same activities query the list
 * uses, so arriving from the list costs only the track request; a direct link
 * fetches both.
 */

import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { activitySplitsQuery, activityTrackQuery, routesQuery } from "../../api/queries";
import type { Activity, ActivitySeriesName, ActivityTrackState } from "../../api/types";
import { routeKey } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import { formatTimestamp } from "../../lib/format";
import { buildActivityProfile, sampleIndexAt } from "../../lib/profile";
import { WHOLE_LAP_COVERAGE } from "../../lib/rideHistory";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { conditionsSentence } from "../../lib/weather";
import { ElevationProfile } from "../routes/ElevationProfile";
import { ActivityMap } from "./ActivityMap";
import { RideConditions, stepStarts } from "./RideConditions";
import { RideFigures } from "./RideFigures";
import { SeriesChips, useRideSeries } from "./RideSeries";
import { RideSplits } from "./RideSplits";
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
  const splits = useQuery({ ...activitySplitsQuery(id ?? 0), enabled: id !== null });
  const coordinates = useMemo(() => track.data?.coordinates ?? [], [track.data]);
  const profile = useMemo(() => buildActivityProfile(coordinates), [coordinates]);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [mapExpanded, setMapExpanded] = useState(false);
  useEscapeKey(mapExpanded, () => setMapExpanded(false));
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
  const weather = track.data?.weather;
  // The strip shares the profile's axis where there is one, and the listed
  // distance — which the splits add up to — where there is not.
  const stripMetres = profile?.totalDistanceMetres ?? ride?.distanceMetres ?? 0;
  // Not until the splits have answered: placed by elapsed time first and by
  // the splits a moment later, the strip would jump.
  const starts = useMemo(
    () =>
      weather && ride && !splits.isPending
        ? stepStarts(
            weather,
            ride.startedAt,
            ride.elapsedSeconds,
            stripMetres,
            splits.data?.splits ?? [],
          )
        : [],
    [weather, ride, stripMetres, splits.isPending, splits.data],
  );
  const drawable = !track.isError && !!track.data?.bbox && coordinates.length >= 2;

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <div className="flex flex-col justify-between gap-6 py-1">
            <div className="flex flex-col gap-1">
              <Link className="text-[var(--ink-2)] text-xs underline" to="/activities">
                Activities
              </Link>
              <h1 className="font-semibold text-4xl tracking-tight">{title}</h1>
              {ride?.weather ? (
                <p className="text-[var(--ink-2)] text-sm">{conditionsSentence(ride.weather)}</p>
              ) : null}
              <MatchedRoute ride={ride} />
            </div>
            <RideFigures ride={ride} />
          </div>
          {id === null ? (
            <p className="self-center text-[var(--ink-2)] text-sm">{absenceMessage(undefined)}</p>
          ) : track.isPending ? (
            <Skeleton
              className="h-80 w-full"
              role="status"
              aria-label="Loading the recorded track"
            />
          ) : !drawable || !track.data?.bbox ? (
            <p className="self-center text-[var(--ink-2)] text-sm">
              {absenceMessage(track.data?.state)}
            </p>
          ) : (
            <div
              className={
                mapExpanded
                  ? "h-[75vh] overflow-hidden rounded-2xl ring-1 ring-black/5 lg:col-span-2"
                  : "h-80 overflow-hidden rounded-2xl ring-1 ring-black/5"
              }
            >
              <ActivityMap
                coordinates={coordinates}
                bounds={track.data.bbox}
                profile={profile}
                activeMetres={activeMetres}
                onActiveChange={setActiveMetres}
                expanded={mapExpanded}
                onExpandedChange={setMapExpanded}
              />
            </div>
          )}
        </div>
        {drawable && profile ? (
          <div className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
            <ElevationProfile
              profile={profile}
              title={title}
              series={drawn}
              activeMetres={activeMetres}
              onActiveChange={setActiveMetres}
              size="tall"
            />
            <RideConditions steps={weather} starts={starts} totalMetres={stripMetres} />
            <SeriesChips
              states={states}
              drawn={drawn}
              activeIndex={activeIndex}
              onToggle={toggle}
            />
          </div>
        ) : weather && weather.length > 0 && ride ? (
          <div className="rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5">
            <RideConditions
              steps={weather}
              starts={starts}
              totalMetres={stripMetres}
              inset={false}
            />
          </div>
        ) : null}
        <TrainingLoad ride={ride} />
        <RideSplits
          splits={splits.data?.splits}
          activeMetres={activeMetres}
          onActiveChange={setActiveMetres}
          {...(profile ? { axisMetres: profile.totalDistanceMetres } : {})}
        />
      </div>
    </PageShell>
  );
}

/** The library route this ride was ridden on. A ride the listing has no route
 * for names nothing, and a partial lap says how much of it it covered. */
function MatchedRoute({ ride }: { ride: Activity | undefined }) {
  const match = ride?.routeMatch;
  // The listing is only worth a request once there is a route to name in it.
  const routes = useQuery({ ...routesQuery(), enabled: match !== undefined });
  const route = match ? routes.data?.find((held) => routeKey(held) === routeKey(match)) : undefined;
  if (!match || !route) {
    return null;
  }

  return (
    <p className="text-sm">
      <Link
        className="underline"
        to={`/routes/${route.provider}/${route.sourceRouteId}/${route.stageOrder}`}
      >
        {route.title}
      </Link>
      {match.routeCoverage >= WHOLE_LAP_COVERAGE ? null : (
        <span className="text-[var(--ink-2)]">
          {" "}
          · {Math.round(match.routeCoverage * 100)}% of the route
        </span>
      )}
    </p>
  );
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
