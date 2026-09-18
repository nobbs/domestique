/** Variants G–I: expanding rows, legs as rows, and a stats header with one committing button. */

import {
  IconArrowsDiagonal,
  IconChevronDown,
  IconChevronUp,
  IconClock,
  IconGripVertical,
  IconMountain,
  IconRoute,
  IconSettings,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "../../../components/Button";
import { Segmented } from "../../../components/Segmented";
import { Popover, PopoverContent, PopoverTrigger } from "../../../components/ui/popover";
import { Switch } from "../../../components/ui/switch";
import { formatCount } from "../../../lib/format";
import { cn } from "../../../lib/utils";
import { CardShell, Section } from "./CardShell";
import { PROFILE_LABEL, type Scenario } from "./fixtures";
import {
  AvoidList,
  HeaderAside,
  PROFILES,
  SaveError,
  StatusLine,
  titleOf,
  WaypointMark,
  WaypointText,
} from "./MoreVariants";

function RouteSettings({ scenario }: { scenario: Scenario }) {
  return (
    <div className="flex flex-col gap-3">
      <Segmented label="Route type" items={PROFILES} value={scenario.profile} onChange={() => {}} />
      <label className="flex items-start gap-2.5 text-sm">
        <Switch className="mt-0.5" checked={scenario.cues} onCheckedChange={() => {}} />
        <span className="flex flex-col">
          Turn cues on Wahoo
          <span className="text-[var(--ink-2)] text-xs">
            {scenario.turnCount === undefined
              ? "Adds turn instructions to the course."
              : `${formatCount(scenario.turnCount, "turn")} from the router.`}
          </span>
        </span>
      </label>
    </div>
  );
}

function Empty() {
  return (
    <p className="py-6 text-center text-[var(--ink-2)] text-sm">
      Click the map to start the route.
    </p>
  );
}

function Footer({ scenario }: { scenario: Scenario }) {
  return (
    <div className="flex flex-col gap-2">
      <StatusLine scenario={scenario} />
      <div className="flex items-center gap-2">
        <Button className="flex-1" disabled={scenario.saving}>
          Save
        </Button>
        <Button variant="outline" disabled={scenario.saving || scenario.waypoints.length < 2}>
          {scenario.published ? "Unpublish" : "Publish"}
        </Button>
      </div>
      <SaveError scenario={scenario} />
    </div>
  );
}

// --- G: a row opens in place to show what can be done to it --------------

export function VariantG({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  const open = count > 2 ? 2 : -1;
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={
        <div className="flex items-center">
          <Popover>
            <PopoverTrigger
              render={
                <Button
                  variant="ghost"
                  icon={<IconSettings size={16} />}
                  aria-label="Route settings"
                />
              }
            />
            <PopoverContent align="end" className="w-72 p-3">
              <RouteSettings scenario={scenario} />
            </PopoverContent>
          </Popover>
          <HeaderAside scenario={scenario} />
        </div>
      }
      footer={<Footer scenario={scenario} />}
    >
      <p className="pb-2 text-[var(--ink-2)] text-xs">
        {PROFILE_LABEL[scenario.profile]} · cues {scenario.cues ? "on" : "off"}
      </p>
      {count === 0 ? (
        <Empty />
      ) : (
        <ol className="flex flex-col gap-1 pb-2">
          {scenario.waypoints.map((waypoint, index) => (
            <li
              key={waypoint.id}
              className={cn(
                "rounded-lg",
                index === open
                  ? "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]"
                  : "hover:bg-[color-mix(in_oklab,var(--ink-2)_5%,transparent)]",
              )}
            >
              <button
                type="button"
                aria-expanded={index === open}
                className="flex w-full items-center gap-2.5 px-1.5 py-1.5 text-left"
              >
                <WaypointMark index={index} count={count} />
                <WaypointText waypoint={waypoint} />
                {index === open ? (
                  <IconChevronUp size={14} className="shrink-0 text-[var(--ink-2)]" />
                ) : (
                  <IconChevronDown size={14} className="shrink-0 text-[var(--ink-2)] opacity-40" />
                )}
              </button>
              {index === open ? (
                <div className="flex items-center gap-3 border-[var(--rule)] border-t px-2 py-2 text-xs">
                  <label className="flex flex-1 items-center gap-2">
                    <Switch checked={Boolean(waypoint.straight)} onCheckedChange={() => {}} />
                    Straight line to here
                  </label>
                  <Button variant="ghost" icon={<IconTrash size={14} />}>
                    Delete
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ol>
      )}
      {scenario.avoid.length > 0 ? (
        <Section
          title="Avoided areas"
          summary={formatCount(scenario.avoid.length, "area")}
          defaultOpen
        >
          <AvoidList scenario={scenario} />
        </Section>
      ) : null}
    </CardShell>
  );
}

// --- H: the legs between waypoints are rows of their own ------------------

export function VariantH({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={<HeaderAside scenario={scenario} />}
      footer={<Footer scenario={scenario} />}
    >
      <Section
        title="Route options"
        summary={`${PROFILE_LABEL[scenario.profile]} · cues ${scenario.cues ? "on" : "off"}`}
        defaultOpen={false}
      >
        <RouteSettings scenario={scenario} />
      </Section>
      {count === 0 ? (
        <Empty />
      ) : (
        <ol className="flex flex-col pt-2 pb-2">
          {scenario.waypoints.map((waypoint, index) => (
            <li key={waypoint.id} className="flex flex-col">
              {index > 0 ? (
                <div className="ml-[11px] flex items-center gap-2 border-l-2 py-1.5 pl-4 text-xs">
                  <span
                    className={cn(
                      "min-w-0 flex-1 truncate text-[var(--ink-2)] tabular-nums",
                      waypoint.straight ? "italic" : undefined,
                    )}
                  >
                    {waypoint.meta?.split(" · ")[0] ?? ""}{" "}
                    {waypoint.straight ? "straight" : "routed"}
                  </span>
                  <Segmented
                    label={`Leg to ${waypoint.name}`}
                    size="sm"
                    items={[
                      { key: "routed", label: "Routed" },
                      { key: "straight", label: "Straight" },
                    ]}
                    value={waypoint.straight ? "straight" : "routed"}
                    onChange={() => {}}
                  />
                </div>
              ) : null}
              <div className="group flex items-center gap-2.5 py-1">
                <WaypointMark index={index} count={count} />
                <span className="min-w-0 flex-1 truncate text-sm">{waypoint.name}</span>
                <IconGripVertical
                  aria-hidden="true"
                  size={14}
                  className="shrink-0 cursor-grab text-[var(--ink-2)] opacity-0 group-hover:opacity-100"
                />
                <Button
                  variant="ghost"
                  icon={<IconTrash size={14} />}
                  aria-label={`Delete ${waypoint.name}`}
                  className="opacity-50 group-hover:opacity-100"
                />
              </div>
            </li>
          ))}
        </ol>
      )}
      {scenario.avoid.length > 0 ? (
        <Section
          title="Avoided areas"
          summary={formatCount(scenario.avoid.length, "area")}
          defaultOpen
        >
          <AvoidList scenario={scenario} />
        </Section>
      ) : null}
    </CardShell>
  );
}

// --- I: the route's own figures first; one button says what saving does ---

export function VariantI({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  const last = scenario.waypoints[count - 1]?.meta?.split(" · ") ?? [];
  const riders = scenario.delivery.total;
  const action = scenario.saving
    ? "Saving…"
    : scenario.published
      ? `Save & update ${formatCount(riders, "rider")}`
      : "Save draft";
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={<HeaderAside scenario={scenario} />}
      footer={
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <Button className="flex-1" disabled={scenario.saving}>
              {action}
            </Button>
            {scenario.published ? null : (
              <Button variant="outline" disabled={count < 2}>
                Publish to Wahoo
              </Button>
            )}
          </div>
          <div className="flex items-center justify-between gap-2">
            <StatusLine scenario={scenario} />
            {scenario.published ? (
              <button type="button" className="shrink-0 text-[var(--ink-2)] text-xs underline">
                Unpublish
              </button>
            ) : null}
          </div>
          <SaveError scenario={scenario} />
        </div>
      }
    >
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
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pb-2 text-xs">
        <Segmented
          label="Route type"
          size="sm"
          items={PROFILES}
          value={scenario.profile}
          onChange={() => {}}
        />
        <label className="flex items-center gap-1.5">
          <Switch checked={scenario.cues} onCheckedChange={() => {}} />
          Cues
        </label>
      </div>
      {count === 0 ? (
        <Empty />
      ) : (
        <ol className="flex flex-col gap-0.5 pb-2">
          {scenario.waypoints.map((waypoint, index) => (
            <li
              key={waypoint.id}
              className="group flex items-center gap-2.5 rounded-lg px-1 py-1.5"
            >
              <WaypointMark index={index} count={count} />
              <WaypointText waypoint={waypoint} />
              {index > 0 ? (
                <Button
                  variant="ghost"
                  aria-pressed={Boolean(waypoint.straight)}
                  icon={<IconArrowsDiagonal size={14} />}
                  aria-label="Straight line to here"
                  className={
                    waypoint.straight ? "text-[var(--accent)]" : "opacity-0 group-hover:opacity-60"
                  }
                />
              ) : null}
            </li>
          ))}
        </ol>
      )}
      {scenario.avoid.length > 0 ? (
        <Section
          title="Avoided areas"
          summary={formatCount(scenario.avoid.length, "area")}
          defaultOpen={false}
        >
          <AvoidList scenario={scenario} />
        </Section>
      ) : null}
    </CardShell>
  );
}
