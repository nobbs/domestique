/**
 * Placeholder shape for per-plan Wahoo delivery status — not the real API.
 * See the task brief in the PR description for the eventual contract.
 */
export type DeliveryState = "current" | "pending" | "outdated" | "failed" | "absent";

export interface PlanTargetDelivery {
  id: string;
  ownerNickname?: string;
  own: boolean;
  state: DeliveryState;
  failure?: "unauthorized" | "quota" | "course" | "state" | "target";
  deliveredAt?: string;
  /** `pending` because the plan was unpublished, not because it is being pushed. */
  removing?: boolean;
}

export interface DeliveryScenario {
  key: string;
  label: string;
  /** Whether the plan being edited is published; an unpublished plan has nothing on Wahoo yet. */
  published: boolean;
  targets: PlanTargetDelivery[];
}

const NOW = "2026-09-18T09:00:00Z";
const RECENT = "2026-09-18T08:45:00Z";
const STALE = "2026-09-16T07:00:00Z";

export const SCENARIOS: DeliveryScenario[] = [
  {
    key: "all-current",
    label: "All current",
    published: true,
    targets: [
      { id: "t1", ownerNickname: "Alexej", own: true, state: "current", deliveredAt: NOW },
      { id: "t2", ownerNickname: "Marta", own: false, state: "current", deliveredAt: RECENT },
      { id: "t3", ownerNickname: "Finn", own: false, state: "current", deliveredAt: RECENT },
    ],
  },
  {
    key: "pushing",
    label: "Push in flight",
    published: true,
    targets: [
      { id: "t1", ownerNickname: "Alexej", own: true, state: "current", deliveredAt: STALE },
      { id: "t2", ownerNickname: "Marta", own: false, state: "pending" },
      { id: "t3", ownerNickname: "Finn", own: false, state: "pending" },
      { id: "t4", ownerNickname: "Jo", own: false, state: "current", deliveredAt: STALE },
    ],
  },
  {
    key: "failures",
    label: "Two failures",
    published: true,
    targets: [
      { id: "t1", ownerNickname: "Alexej", own: true, state: "current", deliveredAt: NOW },
      {
        id: "t2",
        ownerNickname: "Marta",
        own: false,
        state: "failed",
        failure: "unauthorized",
        deliveredAt: STALE,
      },
      { id: "t3", ownerNickname: "Finn", own: false, state: "failed", failure: "quota" },
    ],
  },
  {
    key: "draft",
    label: "Draft (unpublished)",
    published: false,
    targets: [
      { id: "t1", ownerNickname: "Alexej", own: true, state: "absent" },
      { id: "t2", ownerNickname: "Marta", own: false, state: "absent" },
    ],
  },
  {
    key: "unpublished",
    label: "Just unpublished",
    published: false,
    targets: [
      {
        id: "t1",
        ownerNickname: "Alexej",
        own: true,
        state: "pending",
        removing: true,
        deliveredAt: STALE,
      },
      {
        id: "t2",
        ownerNickname: "Marta",
        own: false,
        state: "pending",
        removing: true,
        deliveredAt: STALE,
      },
      { id: "t3", ownerNickname: "Finn", own: false, state: "absent", deliveredAt: STALE },
    ],
  },
  {
    key: "no-nickname",
    label: "Rider with no nickname",
    published: true,
    targets: [
      { id: "t1", ownerNickname: "Alexej", own: true, state: "current", deliveredAt: NOW },
      { id: "t2", own: false, state: "outdated", deliveredAt: STALE },
    ],
  },
  {
    key: "single-rider",
    label: "Single rider",
    published: true,
    targets: [{ id: "t1", ownerNickname: "Alexej", own: true, state: "current", deliveredAt: NOW }],
  },
];

/** A plan that was never published: no target has ever held or been sent a copy. */
export function isTrueDraft(scenario: DeliveryScenario): boolean {
  return (
    !scenario.published &&
    scenario.targets.every((target) => target.state === "absent" && !target.deliveredAt)
  );
}
