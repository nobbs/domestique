/** Where each connected rider's Wahoo account stands with one saved plan. */

import { IconRefresh, IconSend } from "@tabler/icons-react";
import { useQueryClient } from "@tanstack/react-query";
import {
  getGetPlanDeliveryQueryKey,
  type PlanTargetDelivery,
  useGetPlanDelivery,
  useRunTaskArgument,
} from "../../api/generated";
import { TASKS } from "../../api/tasks";
import { Button } from "../../components/Button";
import { InsetList, InsetRow, type RowTone } from "../../components/InsetList";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "../../components/ui/popover";
import { Spinner } from "../../components/ui/spinner";
import { formatCount, formatReadTime } from "../../lib/format";

const POLL_MS = 2000;

const STATE_TONE: Record<PlanTargetDelivery["state"], RowTone> = {
  current: "good",
  pending: "quiet",
  failed: "alert",
  absent: "quiet",
};

const TONE_VAR: Record<RowTone, string> = {
  good: "--good",
  hold: "--hold",
  alert: "--alert",
  quiet: "--ink-2",
};

/** Short forms of `lib/syncGuidance`'s wording, sized for one row. */
const FAILURE_LABEL: Record<string, string> = {
  authorization: "Must reconnect Wahoo",
  destination: "Wahoo could not complete the write",
  course: "Route could not be encoded",
  state: "Stored state error",
  deletion_limit: "Held back by the deletion limit",
};

function failureLabel(failure: string | undefined): string {
  return (failure && FAILURE_LABEL[failure]) || "Push failed";
}

function riderLabel(target: PlanTargetDelivery): string {
  if (target.ownerNickname) {
    return target.own ? `${target.ownerNickname} (you)` : target.ownerNickname;
  }
  return target.own ? "You" : "Unnamed rider";
}

/** Unpublished and held by no rider: nothing to report but what publishing does. */
function isDraft(targets: PlanTargetDelivery[], published: boolean): boolean {
  return !published && targets.every((target) => target.state === "absent");
}

function stateLabel(target: PlanTargetDelivery, removing: boolean): string {
  if (target.state === "pending") {
    return removing ? "Removing…" : "Sending…";
  }
  return {
    current: "On Wahoo",
    pending: "Sending…",
    failed: "Push failed",
    absent: "Not on Wahoo",
  }[target.state];
}

/** The row's own line: what happened, and when — the popover's reason to exist. */
function rowDetail(target: PlanTargetDelivery, removing: boolean): string {
  if (target.state === "failed") {
    return failureLabel(target.failure);
  }
  if (target.state === "pending") {
    return removing ? "Removing…" : "Sending…";
  }
  if (target.state === "current") {
    // Only a push this process made knows when; a full sync's copy is just there.
    return target.deliveredAt ? `Delivered ${formatReadTime(target.deliveredAt)}` : "On Wahoo";
  }
  return "Not on Wahoo";
}

function summaryLine(targets: PlanTargetDelivery[], removing: boolean): string {
  if (removing) {
    return `Removing from ${formatCount(targets.length, "rider")}`;
  }
  const current = targets.filter((target) => target.state === "current").length;
  return `Sent to ${current} of ${formatCount(targets.length, "rider")}`;
}

/** good/alert/hold read the way `InsetRow` already reads them; `null` draws no dot at all. */
function aggregateTone(targets: PlanTargetDelivery[], published: boolean): RowTone | null {
  if (targets.length === 0 || isDraft(targets, published)) {
    return null;
  }
  if (targets.some((target) => target.state === "failed")) {
    return "alert";
  }
  if (targets.some((target) => target.state === "pending")) {
    return "hold";
  }
  if (targets.every((target) => target.state === "current")) {
    return "good";
  }
  return "hold";
}

/** Poll while a push or removal is in flight for this plan; otherwise leave it be. */
export function pollInterval(targets: PlanTargetDelivery[]): number | false {
  return targets.some((target) => target.state === "pending") ? POLL_MS : false;
}

export interface PlanDeliveryTriggerProps {
  /** `null` for an unsaved draft, which has nothing to report yet — the trigger stays hidden. */
  planId: number | null;
  /** Whether the plan is currently published — a pending target reads as a removal when it is not. */
  published: boolean;
}

/** The header's own trigger: a ghost send button with a status dot, and the popover it opens. */
export function PlanDeliveryTrigger({ planId, published }: PlanDeliveryTriggerProps) {
  const queryClient = useQueryClient();
  const queryKey = getGetPlanDeliveryQueryKey(planId ?? 0);
  const delivery = useGetPlanDelivery(planId ?? 0, {
    query: {
      enabled: planId !== null,
      select: (response) => response.data.targets,
      refetchInterval: (query) => {
        const raw = query.state.data as { data?: { targets?: PlanTargetDelivery[] } } | undefined;
        return raw?.data?.targets ? pollInterval(raw.data.targets) : false;
      },
    },
  });
  const retry = useRunTaskArgument({
    mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey }) },
  });

  if (planId === null) {
    return null;
  }

  const targets = delivery.data ?? [];
  const draft = isDraft(targets, published);
  const tone = aggregateTone(targets, published);
  const pending = targets.some((target) => target.state === "pending");
  const removing = !published;

  return (
    <Popover onOpenChange={(open) => open && delivery.refetch()}>
      <span className="relative inline-flex">
        <PopoverTrigger
          render={<Button variant="ghost" icon={<IconSend size={16} stroke={1.8} />} />}
          aria-label="Wahoo delivery status"
        />
        {tone ? (
          <span
            aria-hidden="true"
            className={`plan-delivery-dot pointer-events-none absolute top-0.5 right-0.5 size-2 rounded-full ring-2 ring-[var(--panel)] ${pending ? "animate-pulse" : ""}`}
            style={{ background: `var(${TONE_VAR[tone]})` }}
          />
        ) : null}
      </span>
      <PopoverContent align="start" aria-label="Wahoo delivery status" className="w-80 gap-3 p-3">
        <PopoverHeader>
          <PopoverTitle>On Wahoo</PopoverTitle>
          <PopoverDescription>
            {draft ? "Not published" : summaryLine(targets, removing)}
          </PopoverDescription>
        </PopoverHeader>
        {draft || targets.length === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            {draft
              ? "Publishing sends this plan to every connected rider's Wahoo account."
              : "No rider has connected Wahoo yet."}
          </p>
        ) : (
          <InsetList>
            {targets.map((target) => (
              <InsetRow
                key={target.id}
                tone={STATE_TONE[target.state]}
                toneLabel={stateLabel(target, removing)}
                title={riderLabel(target)}
                detail={rowDetail(target, removing)}
                actions={
                  // Only the rider reconnecting mends an authorization failure.
                  target.state === "failed" && target.failure !== "authorization" ? (
                    <Button
                      variant="ghost"
                      icon={<IconRefresh size={14} />}
                      aria-label={`Retry ${riderLabel(target)}`}
                      disabled={retry.isPending}
                      onClick={() =>
                        retry.mutate({ name: TASKS.syncPlan, argument: String(planId) })
                      }
                    />
                  ) : target.state === "pending" ? (
                    <Spinner aria-label={stateLabel(target, removing)} />
                  ) : null
                }
              />
            ))}
          </InsetList>
        )}
      </PopoverContent>
    </Popover>
  );
}
