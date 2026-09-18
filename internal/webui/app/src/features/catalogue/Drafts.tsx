/** The admin's unpublished plans, shown on the catalogue's Drafts shelf and opened in the planner. */

import { IconChevronRight, IconPencil } from "@tabler/icons-react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { getGetPlanQueryOptions, getListPlansQueryOptions } from "../../api/generated";
import type { PlanSummary, Position, Route } from "../../api/types";
import { ButtonLink } from "../../components/Button";
import { RouteGlyph } from "../../components/RouteGlyph";
import { formatAscent, formatCount, formatDistance } from "../../lib/format";
import { cn } from "../../lib/utils";

const HOUR_MS = 3_600_000;

/** "3 h ago", "yesterday", "8 days ago": how long a draft has sat untouched. */
export function editedAgo(at: string, now = new Date()): string {
  const hours = Math.round((now.getTime() - Date.parse(at)) / HOUR_MS);
  if (!Number.isFinite(hours)) {
    return "";
  }
  if (hours < 24) {
    return hours < 1 ? "just now" : `${hours} h ago`;
  }
  const days = Math.round(hours / 24);

  return days === 1 ? "yesterday" : `${days} days ago`;
}

export interface Draft {
  plan: PlanSummary;
  coordinates: Position[];
}

/** The admin's drafts, newest edit first, each with the line it was last routed along. */
export function useDrafts(enabled: boolean): { drafts: Draft[]; isError: boolean } {
  const plans = useQuery({ ...getListPlansQueryOptions(), enabled });
  const drafts = (plans.data?.data.plans ?? [])
    .filter((plan) => !plan.published)
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  // Under the planner's own keys, so opening a draft from here finds it already read.
  const lines = useQueries({
    queries: drafts.map((plan) => getGetPlanQueryOptions(plan.id)),
  });

  return {
    isError: plans.isError,
    drafts: drafts.map((plan, index) => ({
      plan,
      coordinates: (lines[index]?.data?.data.geometry.coordinates ?? []) as Position[],
    })),
  };
}

/** Where an admin edits a published plan, or nothing for a route this service does not own. */
export function planEditLink(route: Route): string | null {
  return route.provider === "local" ? `/plan/${route.sourceRouteId}` : null;
}

/** The pencil at a published plan's row end; a sibling of the row's own link, never inside it. */
export function EditPlanButton({ route, className }: { route: Route; className?: string }) {
  const to = planEditLink(route);

  return to === null ? null : (
    <ButtonLink
      variant="ghost"
      to={to}
      icon={<IconPencil size={15} />}
      aria-label={`Edit plan ${route.title}`}
      title="Edit plan"
      className={cn("opacity-40 group-hover:opacity-100 focus-visible:opacity-100", className)}
    />
  );
}

/** A draft as a ledger row: its shape, name, the figures a draft has, and when it was last edited. */
function DraftRow({ draft }: { draft: Draft }) {
  const { plan } = draft;

  return (
    <li className="border-[var(--panel)] border-b-2 last:border-b-0">
      <Link
        to={`/plan/${plan.id}`}
        className="relative grid grid-cols-[2.5rem_minmax(0,1fr)_5.5rem_5rem_6.5rem_1rem] items-center gap-x-4 px-3 py-2.5 text-sm tabular-nums before:absolute before:inset-1 before:rounded-[7px] hover:before:bg-[color-mix(in_oklab,var(--ink-2)_8%,transparent)]"
      >
        <span className="relative block size-10">
          <RouteGlyph coordinates={draft.coordinates} title={plan.name} band={0} />
        </span>
        <span className="relative flex min-w-0 flex-col gap-0.5">
          <span className="truncate font-semibold">{plan.name}</span>
          <span className="text-[var(--ink-2)] text-xs">
            {formatCount(plan.waypointCount, "waypoint")}
          </span>
        </span>
        <span className="relative text-right font-semibold">
          {formatDistance(plan.distanceMetres)}
        </span>
        <span className="relative text-right">{formatAscent(plan.ascentMetres)}</span>
        <span className="relative text-right text-[var(--ink-2)]">{editedAgo(plan.updatedAt)}</span>
        <IconChevronRight aria-hidden="true" size={14} className="relative text-[var(--ink-2)]" />
      </Link>
    </li>
  );
}

/** A draft as a card, where a ledger row will not fit. */
function DraftCard({ draft }: { draft: Draft }) {
  const { plan } = draft;

  return (
    <li>
      <Link
        to={`/plan/${plan.id}`}
        className="flex items-start gap-3 rounded-lg border border-[var(--rule)] p-3 hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
      >
        <span className="mt-0.5 block size-10 shrink-0">
          <RouteGlyph coordinates={draft.coordinates} title={plan.name} band={0} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block font-semibold">{plan.name}</span>
          <span className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
            <span>{formatDistance(plan.distanceMetres)}</span>
            <span>{formatAscent(plan.ascentMetres)}</span>
            <span>{formatCount(plan.waypointCount, "waypoint")}</span>
            <span>{editedAgo(plan.updatedAt)}</span>
          </span>
        </span>
      </Link>
    </li>
  );
}

export function DraftList({ drafts, narrow }: { drafts: Draft[]; narrow: boolean }) {
  if (drafts.length === 0) {
    return (
      <p className="py-6 text-center text-[var(--ink-2)] text-sm">
        No drafts. A plan stays here until it is published.
      </p>
    );
  }

  return narrow ? (
    <ul className="grid gap-2">
      {drafts.map((draft) => (
        <DraftCard key={draft.plan.id} draft={draft} />
      ))}
    </ul>
  ) : (
    <ul className="flex flex-col overflow-hidden rounded-[11px] bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
      {drafts.map((draft) => (
        <DraftRow key={draft.plan.id} draft={draft} />
      ))}
    </ul>
  );
}
