/** Variant J: the mix the user picked — E's footer, a plain name field, legs as glyphs on the line. */

import {
  IconChevronDown,
  IconClock,
  IconGripVertical,
  IconMountain,
  IconPencil,
  IconPlus,
  IconRoute,
  IconRoute2,
  IconSend,
  IconTrash,
} from "@tabler/icons-react";
import { useState } from "react";
import { Button } from "../../../components/Button";
import { Segmented } from "../../../components/Segmented";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../../../components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../../components/ui/dropdown-menu";
import { formatCount } from "../../../lib/format";
import { statusDot } from "./CardShell";
import { PROFILE_LABEL, type Scenario } from "./fixtures";
import { DELIVERY_TONE, Pill, PROFILES, SaveError, WaypointMark } from "./MoreVariants";

/** The plan mark doubles as the plans menu: start a new plan, or delete this one. */
function PlansMenu({ scenario, deleting }: { scenario: Scenario; deleting: boolean }) {
  const [confirming, setConfirming] = useState(deleting);
  const saved = scenario.planName !== "";
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              aria-label="Plans"
              className="group relative grid size-9 shrink-0 place-items-center rounded-md [background:var(--mark)] text-[var(--mark-ink)]"
            />
          }
        >
          <IconRoute size={18} stroke={1.8} />
          <span className="absolute -right-1 -bottom-1 grid size-4 place-items-center rounded-full bg-[var(--panel)] text-[var(--ink)] shadow-sm ring-1 ring-[var(--rule)]">
            <IconChevronDown size={10} stroke={2.5} />
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-60">
          <DropdownMenuItem>
            <IconPlus size={14} /> New plan
          </DropdownMenuItem>
          {saved ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onClick={() => setConfirming(true)}>
                <IconTrash size={14} /> Delete this plan…
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete “{scenario.planName}”?</AlertDialogTitle>
            <AlertDialogDescription>
              {scenario.published
                ? `It is removed from Wahoo for ${formatCount(scenario.delivery.total, "rider")} straight away. This cannot be undone.`
                : "It is a draft, so nothing is on Wahoo. This cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep it</AlertDialogCancel>
            <AlertDialogAction variant="destructive">Delete plan</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function NameField({ scenario, onEdit }: { scenario: Scenario; onEdit: () => void }) {
  return (
    <label className="group flex min-w-0 flex-1 items-center gap-1.5 rounded-lg border border-[var(--rule)] bg-[var(--base)] px-2.5 py-1.5 focus-within:border-[var(--accent)] hover:border-[color-mix(in_oklab,var(--ink-2)_40%,var(--rule))]">
      <input
        aria-label="Plan name"
        defaultValue={scenario.planName}
        onChange={onEdit}
        placeholder="Name this route"
        className="min-w-0 flex-1 truncate bg-transparent font-semibold text-base outline-none placeholder:font-normal placeholder:text-[var(--ink-2)]"
      />
      <IconPencil aria-hidden="true" size={14} className="shrink-0 text-[var(--ink-2)]" />
    </label>
  );
}

/** How a leg's stroke is drawn; the spike compares these side by side. */
export type LegStyle = "hairline" | "solid" | "dashed" | "dotted" | "mixed" | "dotted-mixed";

/** How a routed leg's line bends; a straight leg is always a straight stroke. */
export type LegShape = "wave" | "wide-wave" | "s-curve" | "zigzag";

const ROUTED_PATHS: Record<LegShape, string> = {
  wave: "M12 0 C16 3 8 7 12 10 C16 13 8 17 12 20",
  "wide-wave": "M12 0 C19 2.5 5 7.5 12 10 C19 12.5 5 17.5 12 20",
  "s-curve": "M12 0 C20 4 20 8 12 10 C4 12 4 16 12 20",
  zigzag: "M12 0 L17 4 L7 10 L17 16 L12 20",
};

const STROKES: Record<LegStyle, { width: number; dash?: string; cap?: "round"; tone: string }> = {
  hairline: { width: 1.25, tone: "35%" },
  solid: { width: 2.5, tone: "45%" },
  dashed: { width: 2.5, dash: "3.5 2.5", tone: "50%" },
  dotted: { width: 2.5, dash: "0.1 4", cap: "round", tone: "40%" },
  mixed: { width: 2.5, tone: "50%" },
  "dotted-mixed": { width: 2.5, cap: "round", tone: "40%" },
};

/** The leg arriving at a waypoint, drawn as the connection itself between the two marks:
 * a stroke where it is straight, a squiggle where it is routed. A click switches them. */
function Leg({
  straight,
  to,
  look,
  shape,
  tall,
  onToggle,
}: {
  straight: boolean;
  to: string;
  look: LegStyle;
  shape: LegShape;
  tall: boolean;
  onToggle?: () => void;
}) {
  const stroke = STROKES[look];
  // Mixed: a routed leg is dashed, a straight one solid.
  const dash =
    look === "mixed"
      ? straight
        ? undefined
        : "3.5 2.5"
      : look === "dotted-mixed"
        ? straight
          ? undefined
          : "0.1 4"
        : stroke.dash;
  return (
    <button
      type="button"
      aria-pressed={straight}
      onClick={onToggle}
      aria-label={straight ? `Route the leg to ${to}` : `Draw the leg to ${to} straight`}
      title={straight ? "Straight — click to route" : "Routed — click to draw straight"}
      className={`absolute left-0 z-10 flex w-6 justify-center hover:!text-[var(--accent)] ${tall ? "top-[-14px] h-7" : "top-[-10px] h-5"}`}
      style={{ color: `color-mix(in oklab, var(--ink-2) ${stroke.tone}, transparent)` }}
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 24 20"
        preserveAspectRatio="none"
        className="h-full w-6 overflow-visible"
      >
        <path
          d={straight ? "M12 0 V20" : ROUTED_PATHS[shape]}
          vectorEffect="non-scaling-stroke"
          strokeLinejoin="round"
          fill="none"
          stroke="currentColor"
          strokeWidth={stroke.width}
          strokeDasharray={dash}
          strokeLinecap={stroke.cap}
        />
      </svg>
    </button>
  );
}

/** An avoided area drawn as its own shape, as the map shows it; only circles exist so far. */
function AreaShape() {
  return (
    <svg aria-hidden="true" viewBox="0 0 14 14" className="size-3.5 shrink-0 text-[var(--alert)]">
      <circle
        cx="7"
        cy="7"
        r="5.5"
        fill="currentColor"
        fillOpacity="0.18"
        stroke="currentColor"
        strokeWidth="1.5"
      />
    </svg>
  );
}

/** Avoided areas sit on the footer as one line and open upwards, leaving the list its room. */
function AvoidDrawer({ scenario }: { scenario: Scenario }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mx-5 mb-1 flex shrink-0 flex-col rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3">
      {open ? (
        <div className="max-h-40 overflow-y-auto pt-2">
          <ul className="flex flex-col gap-1">
            {scenario.avoid.map((area) => (
              <li key={area.id} className="flex items-center gap-2 text-xs">
                <AreaShape />
                <span className="min-w-0 flex-1 truncate text-[var(--ink-2)]">{area.place}</span>
                <span className="shrink-0 tabular-nums">{area.radius}</span>
                <Button
                  variant="ghost"
                  icon={<IconTrash size={14} />}
                  aria-label={`Delete ${area.place}`}
                />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 py-2 text-left text-sm"
      >
        <span className="flex-1 font-medium">Avoided areas</span>
        <span className="text-[var(--ink-2)] text-xs">
          {formatCount(scenario.avoid.length, "area")}
        </span>
        <IconChevronDown
          aria-hidden="true"
          size={14}
          className={`shrink-0 text-[var(--ink-2)] transition-transform ${open ? "" : "rotate-180"}`}
        />
      </button>
    </div>
  );
}

/** Short plans show every waypoint; only a longer one folds. */
const COLLAPSE_ABOVE = 7;

type Segment = { kind: "row"; index: number } | { kind: "gap"; from: number; to: number };

/** Rows to show and the hidden runs between them, in order. */
function segments(count: number, shown: (index: number) => boolean): Segment[] {
  const out: Segment[] = [];
  for (let index = 0; index < count; index++) {
    if (shown(index)) {
      out.push({ kind: "row", index });
      continue;
    }
    const last = out[out.length - 1];
    if (last?.kind === "gap") {
      last.to = index;
    } else {
      out.push({ kind: "gap", from: index, to: index });
    }
  }
  return out;
}

/** The distance a hidden run covers, read from the running totals either side of it. */
function gapKilometres(scenario: Scenario, from: number, to: number): string {
  const km = (index: number) => Number.parseFloat(scenario.waypoints[index]?.meta ?? "0") || 0;
  return `${(km(to) - km(from - 1)).toFixed(1)} km`;
}

export function VariantJ({
  scenario,
  look = "hairline",
  shape = "wave",
  tall = false,
  deleting = false,
  collapse = false,
  focus = -1,
  neighbours = 1,
  dragging = false,
}: {
  collapse?: boolean;
  focus?: number;
  neighbours?: number;
  dragging?: boolean;
  scenario: Scenario;
  deleting?: boolean;
  look?: LegStyle;
  shape?: LegShape;
  tall?: boolean;
}) {
  const count = scenario.waypoints.length;
  const [published, setPublished] = useState(scenario.published);
  const [straight, setStraight] = useState(
    () => new Set(scenario.waypoints.filter((waypoint) => waypoint.straight).map((w) => w.id)),
  );
  const [profile, setProfile] = useState(scenario.profile);
  const [cues, setCues] = useState(scenario.cues);
  const [edited, setEdited] = useState(false);
  const [opened, setOpened] = useState<Set<number>>(() => new Set());
  // A new focus folds whatever was opened, so the list follows the work on the map.
  const [openedFor, setOpenedFor] = useState(focus);
  if (openedFor !== focus) {
    setOpenedFor(focus);
    setOpened(new Set());
  }
  const folding = collapse && count > COLLAPSE_ABOVE;
  // Always the ends and the waypoint last touched on the map with its neighbours; a drag shows all.
  const shown = (index: number) =>
    !folding ||
    dragging ||
    index === 0 ||
    index === count - 1 ||
    (focus >= 0 && Math.abs(index - focus) <= neighbours) ||
    [...opened].some((from) => index >= from && hiddenRunEnd(from) >= index);
  const hiddenRunEnd = (from: number) => {
    let end = from;
    while (end + 1 < count - 1 && !(focus >= 0 && Math.abs(end + 1 - focus) <= neighbours)) {
      end++;
    }
    return end;
  };
  const changed = (apply: () => void) => {
    apply();
    setEdited(true);
  };
  const last = scenario.waypoints[count - 1]?.meta?.split(" · ") ?? [];
  // Legs are keyed by the waypoint they arrive at, so a run's legs end at the row after it.
  const straightIn = (from: number, through: number) =>
    scenario.waypoints.slice(from, through + 1).filter((waypoint) => straight.has(waypoint.id))
      .length;
  return (
    <section className="flex h-[640px] w-[22.5rem] min-w-0 flex-col rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]">
      <div className="flex shrink-0 items-center gap-2.5 px-5 pt-5 pb-3">
        <PlansMenu scenario={scenario} deleting={deleting} />
        <NameField scenario={scenario} onEdit={() => setEdited(true)} />
        <span className="relative inline-flex shrink-0 p-1.5" title="On Wahoo">
          <IconSend size={17} stroke={1.8} className="text-[var(--ink-2)]" />
          {scenario.delivery.state === "none" ? null : (
            <span className="absolute top-0.5 right-0.5">
              {statusDot(
                DELIVERY_TONE[scenario.delivery.state],
                scenario.delivery.state === "pending",
              )}
            </span>
          )}
        </span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-5">
        {count > 1 ? (
          <dl className="mb-3 grid grid-cols-3 gap-2 rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3 py-2 text-center">
            <div>
              <dt className="flex items-center justify-center gap-1 text-[10px] text-[var(--ink-2)] uppercase">
                <IconRoute size={11} /> Distance
              </dt>
              <dd className="font-semibold text-sm tabular-nums">{last[0] ?? "—"}</dd>
            </div>
            <div>
              <dt className="flex items-center justify-center gap-1 text-[10px] text-[var(--ink-2)] uppercase">
                <IconMountain size={11} /> Climb
              </dt>
              <dd className="font-semibold text-sm tabular-nums">412 m</dd>
            </div>
            <div>
              <dt className="flex items-center justify-center gap-1 text-[10px] text-[var(--ink-2)] uppercase">
                <IconClock size={11} /> Time
              </dt>
              <dd className="font-semibold text-sm tabular-nums">{last[1] ?? "—"}</dd>
            </div>
          </dl>
        ) : null}
        <div className="flex flex-wrap gap-1.5 pb-3">
          <DropdownMenu>
            <DropdownMenuTrigger render={<Pill />}>
              <IconRoute2 size={13} />
              {PROFILE_LABEL[profile]}
              <IconChevronDown size={12} />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {PROFILES.map((option) => (
                <DropdownMenuItem
                  key={option.key}
                  onClick={() => changed(() => setProfile(option.key))}
                >
                  {option.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <Pill
            pressed={cues}
            title="Turn cues on Wahoo"
            onClick={() => changed(() => setCues(!cues))}
          >
            Turn cues
            {cues && scenario.turnCount !== undefined ? ` · ${scenario.turnCount}` : ""}
          </Pill>
        </div>
        {count === 0 ? (
          <p className="py-6 text-center text-[var(--ink-2)] text-sm">
            Click the map to start the route.
          </p>
        ) : (
          <ol className="flex flex-col pb-2">
            {segments(count, shown).map((segment, position, all) => {
              if (segment.kind === "gap") {
                return (
                  <li key={`gap-${segment.from}`} className="relative flex h-8 items-center">
                    <span
                      aria-hidden="true"
                      className="absolute top-[-10px] bottom-[-10px] left-[11px] border-[color-mix(in_oklab,var(--ink-2)_35%,transparent)] border-l-2 border-dotted"
                    />
                    <button
                      type="button"
                      onClick={() => setOpened((current) => new Set(current).add(segment.from))}
                      className="ml-[34px] flex min-w-0 flex-1 items-center gap-1.5 rounded-md px-2 py-1 text-left text-[var(--ink-2)] text-xs hover:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]"
                    >
                      <IconChevronDown size={12} />
                      {formatCount(segment.to - segment.from + 1, "more waypoint")}
                      {` · ${gapKilometres(scenario, segment.from, segment.to)}`}
                      {straightIn(segment.from, segment.to + 1) > 0
                        ? ` · ${straightIn(segment.from, segment.to + 1)} straight`
                        : ""}
                    </button>
                  </li>
                );
              }
              const index = segment.index;
              const waypoint = scenario.waypoints[index];
              // The leg into a row after a hidden run starts at a hidden waypoint; the run's line stands for it.
              const afterGap = all[position - 1]?.kind === "gap";
              if (!waypoint) {
                return null;
              }
              return (
                <li
                  key={waypoint.id}
                  className={`group relative flex items-center gap-2.5 rounded-lg ${tall ? "h-[52px]" : "h-11"}`}
                >
                  {index > 0 && !afterGap ? (
                    <Leg
                      straight={straight.has(waypoint.id)}
                      onToggle={() =>
                        changed(() =>
                          setStraight((current) => {
                            const next = new Set(current);
                            next.has(waypoint.id)
                              ? next.delete(waypoint.id)
                              : next.add(waypoint.id);
                            return next;
                          }),
                        )
                      }
                      to={waypoint.name}
                      look={look}
                      shape={shape}
                      tall={tall}
                    />
                  ) : null}
                  <span
                    className={
                      index === focus
                        ? "rounded-full ring-2 ring-[var(--accent)] ring-offset-2 ring-offset-[var(--panel)]"
                        : undefined
                    }
                  >
                    <WaypointMark index={index} count={count} />
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className={`truncate text-sm ${index === focus ? "font-semibold" : ""}`}>
                      {waypoint.name}
                    </span>
                    {waypoint.meta ? (
                      <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                        {waypoint.meta}
                      </span>
                    ) : null}
                  </span>
                  <IconGripVertical
                    aria-label={`Drag ${waypoint.name}`}
                    size={15}
                    className="shrink-0 cursor-grab text-[var(--ink-2)]"
                  />
                  <Button
                    variant="ghost"
                    icon={<IconTrash size={15} />}
                    aria-label={`Delete ${waypoint.name}`}
                  />
                </li>
              );
            })}
          </ol>
        )}
      </div>
      {scenario.avoid.length > 0 ? <AvoidDrawer scenario={scenario} /> : null}
      <div className="shrink-0 p-3">
        <div className="flex items-center gap-2">
          <Segmented
            label="Publication"
            size="sm"
            items={[
              { key: "draft", label: "Draft", fill: "var(--hold)" },
              { key: "published", label: "Published", fill: "var(--good)" },
            ]}
            value={published ? "published" : "draft"}
            onChange={(next) => changed(() => setPublished(next === "published"))}
          />
          <Button
            variant={edited ? "default" : "outline"}
            className="flex-1"
            disabled={scenario.saving || !edited}
          >
            {scenario.saving ? "Saving…" : "Save"}
          </Button>
        </div>
        <SaveError scenario={scenario} />
      </div>
    </section>
  );
}
