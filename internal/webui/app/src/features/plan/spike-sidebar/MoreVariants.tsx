/** Variants D–F: header keeps Plans and New; names ellipsise; route first, settings second. */

import {
  IconBan,
  IconChevronDown,
  IconFlagCheck,
  IconGripVertical,
  IconLine,
  IconPlayerPlay,
  IconRoute2,
  IconSend,
  IconSettings,
  IconTrash,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button } from "../../../components/Button";
import { Segmented } from "../../../components/Segmented";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../../components/ui/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "../../../components/ui/popover";
import { Switch } from "../../../components/ui/switch";
import { formatCount } from "../../../lib/format";
import { cn } from "../../../lib/utils";
import { CardShell, Section, statusDot } from "./CardShell";
import { PROFILE_LABEL, type Scenario, type SpikeWaypoint } from "./fixtures";

export const PROFILES = [
  { key: "trekking", label: "Trekking" },
  { key: "fastbike", label: "Road" },
  { key: "gravel", label: "Gravel" },
] as const;

export const DELIVERY_TONE: Record<Scenario["delivery"]["state"], string> = {
  none: "--ink-2",
  current: "--good",
  pending: "--hold",
  failed: "--alert",
};

export function deliveryText(scenario: Scenario): string {
  const { state, sent, total } = scenario.delivery;
  switch (state) {
    case "none":
      return scenario.published ? "Published" : "Draft · not on Wahoo";
    case "failed":
      return `On Wahoo for ${sent} of ${formatCount(total, "rider")} · retry`;
    case "pending":
      return "Sending to Wahoo…";
    default:
      return `On Wahoo for ${sent} of ${formatCount(total, "rider")}`;
  }
}

/** The header every variant here shares: name, then Plans, New and the Wahoo status. */
export function HeaderAside({ scenario }: { scenario: Scenario }) {
  return (
    <div className="flex items-center gap-0.5">
      <Button variant="ghost" icon={<IconChevronDown size={14} />}>
        Plans
      </Button>
      <Button variant="ghost">New</Button>
      <span className="relative inline-flex p-1.5">
        <IconSend size={16} stroke={1.8} className="text-[var(--ink-2)]" />
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
  );
}

export function titleOf(scenario: Scenario): string {
  return scenario.planName === "" ? "Plan a route" : scenario.planName;
}

export function WaypointMark({ index, count }: { index: number; count: number }) {
  const terminal = index === 0 || index === count - 1;
  return (
    <span
      aria-hidden="true"
      className={cn(
        "relative z-10 grid size-6 shrink-0 place-items-center rounded-full font-semibold text-white text-xs ring-2 ring-[var(--panel)]",
        terminal ? "bg-[var(--accent)]" : "bg-[var(--ink-2)]",
      )}
    >
      {index === 0 ? (
        <IconPlayerPlay size={12} stroke={3} />
      ) : index === count - 1 ? (
        <IconFlagCheck size={13} stroke={2.5} />
      ) : (
        index + 1
      )}
    </span>
  );
}

export function WaypointText({ waypoint }: { waypoint: SpikeWaypoint }) {
  return (
    <span className="flex min-w-0 flex-1 flex-col">
      <span className="truncate text-sm">{waypoint.name}</span>
      <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
        {waypoint.stop}
        {waypoint.meta ? ` · ${waypoint.meta}` : ""}
      </span>
    </span>
  );
}

export function PublishControl({ scenario }: { scenario: Scenario }) {
  if (!scenario.published) {
    return (
      <Button variant="outline" disabled={scenario.saving || scenario.waypoints.length < 2}>
        Publish
      </Button>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" icon={<IconChevronDown size={14} />} />}
      >
        Published
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem variant="destructive">Unpublish — removes from Wahoo</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function StatusLine({ scenario }: { scenario: Scenario }) {
  return (
    <p className="flex min-w-0 items-center gap-1.5 text-xs">
      {statusDot(DELIVERY_TONE[scenario.delivery.state], scenario.delivery.state === "pending")}
      <span
        className="truncate"
        style={{ color: `var(${DELIVERY_TONE[scenario.delivery.state]})` }}
      >
        {scenario.saving ? "Saving…" : deliveryText(scenario)}
      </span>
    </p>
  );
}

export function SaveError({ scenario }: { scenario: Scenario }) {
  return scenario.saveError ? (
    <p className="text-[var(--alert)] text-xs">{scenario.saveError}</p>
  ) : null;
}

export function AvoidList({ scenario }: { scenario: Scenario }) {
  return (
    <ul className="flex flex-col gap-1">
      {scenario.avoid.map((area) => (
        <li key={area.id} className="flex items-center gap-2 text-xs">
          <IconBan aria-hidden="true" size={14} className="shrink-0 text-[var(--alert)]" />
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
  );
}

export function Pill({
  pressed,
  children,
  ...rest
}: { pressed?: boolean; children?: ReactNode } & React.ComponentProps<"button">) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      className={cn(
        "inline-flex h-8 items-center gap-1.5 rounded-[9px] border px-2.5 text-xs",
        pressed
          ? "border-transparent bg-[color-mix(in_oklab,var(--accent)_16%,transparent)] text-[var(--accent)]"
          : "border-[var(--rule)] text-[var(--ink)]",
      )}
      {...rest}
    >
      {children}
    </button>
  );
}

// --- D: a timeline, settings as pills ------------------------------------

export function VariantD({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={<HeaderAside scenario={scenario} />}
      footer={
        <div className="flex flex-col gap-2">
          <StatusLine scenario={scenario} />
          <div className="flex items-center gap-2">
            <Button className="flex-1" disabled={scenario.saving}>
              Save
            </Button>
            <PublishControl scenario={scenario} />
          </div>
          <SaveError scenario={scenario} />
        </div>
      }
    >
      <div className="flex flex-wrap gap-1.5 pb-3">
        <DropdownMenu>
          <DropdownMenuTrigger render={<Pill />}>
            <IconRoute2 size={13} />
            {PROFILE_LABEL[scenario.profile]}
            <IconChevronDown size={12} />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {PROFILES.map((profile) => (
              <DropdownMenuItem key={profile.key}>{profile.label}</DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Pill pressed={scenario.cues} title="Turn cues on Wahoo">
          Turn cues
          {scenario.cues && scenario.turnCount !== undefined ? ` · ${scenario.turnCount}` : ""}
        </Pill>
        {scenario.avoid.length > 0 ? (
          <Pill>
            <IconBan size={13} className="text-[var(--alert)]" />
            {formatCount(scenario.avoid.length, "avoided area")}
          </Pill>
        ) : null}
      </div>

      {count === 0 ? (
        <p className="py-6 text-center text-[var(--ink-2)] text-sm">
          Click the map to start the route.
        </p>
      ) : (
        <ol className="relative flex flex-col pb-2">
          {scenario.waypoints.map((waypoint, index) => (
            <li key={waypoint.id} className="group relative flex items-center gap-3 py-1.5">
              {index > 0 ? (
                <span
                  aria-hidden="true"
                  className={cn(
                    "absolute top-0 left-[11px] h-1/2 w-0.5",
                    waypoint.straight
                      ? "bg-[repeating-linear-gradient(to_bottom,var(--ink-2)_0_3px,transparent_3px_6px)]"
                      : "bg-[var(--rule)]",
                  )}
                />
              ) : null}
              {index < count - 1 ? (
                <span
                  aria-hidden="true"
                  className={cn(
                    "absolute bottom-0 left-[11px] h-1/2 w-0.5",
                    scenario.waypoints[index + 1]?.straight
                      ? "bg-[repeating-linear-gradient(to_bottom,var(--ink-2)_0_3px,transparent_3px_6px)]"
                      : "bg-[var(--rule)]",
                  )}
                />
              ) : null}
              <WaypointMark index={index} count={count} />
              <WaypointText waypoint={waypoint} />
              <IconGripVertical
                aria-hidden="true"
                size={14}
                className="shrink-0 cursor-grab text-[var(--ink-2)] opacity-0 group-hover:opacity-100"
              />
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant="ghost"
                      icon={<IconChevronDown size={14} />}
                      aria-label={`Actions for ${waypoint.name}`}
                      className="opacity-60 group-hover:opacity-100"
                    />
                  }
                />
                <DropdownMenuContent align="end">
                  {index === 0 ? null : (
                    <DropdownMenuItem>
                      {waypoint.straight ? "Route this leg" : "Straight line to here"}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem variant="destructive">Delete</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </li>
          ))}
        </ol>
      )}

      {scenario.avoid.length > 0 ? (
        <div className="border-[var(--rule)] border-t py-3">
          <AvoidList scenario={scenario} />
        </div>
      ) : null}
    </CardShell>
  );
}

// --- E: settings behind one summary popover; the badge is the row menu ----

export function VariantE({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  const last = scenario.waypoints[count - 1];
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={<HeaderAside scenario={scenario} />}
      footer={
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <Segmented
              label="Publication"
              items={[
                { key: "draft", label: "Draft" },
                { key: "published", label: "Published" },
              ]}
              value={scenario.published ? "published" : "draft"}
              onChange={() => {}}
            />
            <Button className="flex-1" disabled={scenario.saving}>
              {scenario.saving ? "Saving…" : "Save"}
            </Button>
          </div>
          <StatusLine scenario={scenario} />
          <SaveError scenario={scenario} />
        </div>
      }
    >
      <Popover>
        <PopoverTrigger
          render={
            <button
              type="button"
              className="mb-3 flex w-full items-center gap-2 rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3 py-2 text-left text-xs"
            />
          }
        >
          <IconSettings size={14} className="shrink-0 text-[var(--ink-2)]" />
          <span className="min-w-0 flex-1 truncate">
            {PROFILE_LABEL[scenario.profile]} · cues {scenario.cues ? "on" : "off"}
            {scenario.avoid.length > 0
              ? ` · ${formatCount(scenario.avoid.length, "avoided area")}`
              : ""}
          </span>
          <span className="shrink-0 text-[var(--ink-2)] tabular-nums">
            {last?.meta?.split(" · ")[0] ?? ""}
          </span>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-72 gap-3 p-3">
          <Segmented
            label="Route type"
            items={PROFILES}
            value={scenario.profile}
            onChange={() => {}}
          />
          <label className="flex items-start gap-2.5 text-sm">
            <Switch className="mt-0.5" checked={scenario.cues} onCheckedChange={() => {}} />
            <span className="flex flex-col">
              Turn cues on Wahoo
              <span className="text-[var(--ink-2)] text-xs">
                Adds turn instructions to the course.
                {scenario.turnCount === undefined
                  ? ""
                  : ` ${formatCount(scenario.turnCount, "turn")}.`}
              </span>
            </span>
          </label>
        </PopoverContent>
      </Popover>

      {count === 0 ? (
        <p className="py-6 text-center text-[var(--ink-2)] text-sm">
          Click the map to start the route.
        </p>
      ) : (
        <ol className="flex flex-col gap-0.5 pb-2">
          {scenario.waypoints.map((waypoint, index) => (
            <li key={waypoint.id} className="flex items-center gap-2.5 rounded-lg px-1 py-1.5">
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <button
                      type="button"
                      aria-label={`Actions for ${waypoint.name}`}
                      className="rounded-full"
                    />
                  }
                >
                  <WaypointMark index={index} count={count} />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  {index === 0 ? null : (
                    <DropdownMenuItem>
                      {waypoint.straight ? "Route this leg" : "Straight line to here"}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem variant="destructive">Delete</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <WaypointText waypoint={waypoint} />
              {waypoint.straight ? (
                <IconLine
                  size={14}
                  className="shrink-0 text-[var(--ink-2)]"
                  aria-label="Straight line"
                />
              ) : null}
              <IconGripVertical
                aria-hidden="true"
                size={14}
                className="shrink-0 cursor-grab text-[var(--ink-2)] opacity-40"
              />
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

// --- F: clean rows; the selected waypoint's actions in one bar ------------

export function VariantF({ scenario }: { scenario: Scenario }) {
  const count = scenario.waypoints.length;
  // The spike shows a selection so the bar is visible; the real one follows a click.
  const selected = count > 2 ? 2 : -1;
  const chosen = scenario.waypoints[selected];
  return (
    <CardShell
      title={titleOf(scenario)}
      aside={<HeaderAside scenario={scenario} />}
      footer={
        <div className="flex flex-col gap-2">
          {chosen ? (
            <div className="flex items-center gap-1.5 rounded-lg bg-[color-mix(in_oklab,var(--accent)_10%,transparent)] px-2 py-1.5 text-xs">
              <span className="min-w-0 flex-1 truncate font-medium">{chosen.stop}</span>
              <Button variant="ghost" icon={<IconLine size={14} />}>
                {chosen.straight ? "Route leg" : "Straight line"}
              </Button>
              <Button variant="ghost" icon={<IconTrash size={14} />} aria-label="Delete waypoint" />
            </div>
          ) : null}
          <StatusLine scenario={scenario} />
          <div className="flex items-center gap-2">
            <Button className="flex-1" disabled={scenario.saving}>
              Save
            </Button>
            <PublishControl scenario={scenario} />
          </div>
          <SaveError scenario={scenario} />
        </div>
      }
    >
      {count === 0 ? (
        <p className="py-6 text-center text-[var(--ink-2)] text-sm">
          Click the map to start the route.
        </p>
      ) : (
        <ol className="flex flex-col gap-0.5 pb-3">
          {scenario.waypoints.map((waypoint, index) => (
            <li
              key={waypoint.id}
              aria-current={index === selected ? "true" : undefined}
              className={cn(
                "group flex items-center gap-2.5 rounded-lg px-1.5 py-1.5",
                index === selected
                  ? "bg-[color-mix(in_oklab,var(--accent)_10%,transparent)]"
                  : "hover:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]",
              )}
            >
              <WaypointMark index={index} count={count} />
              <WaypointText waypoint={waypoint} />
              {waypoint.straight ? (
                <span className="shrink-0 text-[10px] text-[var(--ink-2)] uppercase tracking-wide">
                  straight
                </span>
              ) : null}
              <IconGripVertical
                aria-hidden="true"
                size={14}
                className="shrink-0 cursor-grab text-[var(--ink-2)] opacity-0 group-hover:opacity-100"
              />
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
      <Section
        title="Route options"
        summary={`${PROFILE_LABEL[scenario.profile]} · cues ${scenario.cues ? `on · ${scenario.turnCount ?? 0}` : "off"}`}
        defaultOpen={false}
      >
        <div className="flex flex-col gap-3">
          <Segmented
            label="Route type"
            items={PROFILES}
            value={scenario.profile}
            onChange={() => {}}
          />
          <label className="flex items-center gap-2.5 text-sm">
            <Switch checked={scenario.cues} onCheckedChange={() => {}} />
            Turn cues on Wahoo
          </label>
        </div>
      </Section>
    </CardShell>
  );
}
