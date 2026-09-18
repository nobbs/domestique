/** Variant A: row actions tucked behind a "⋯" menu; footer keeps Save and Publish as two buttons. */

import {
  IconBan,
  IconDots,
  IconFlagCheck,
  IconGripVertical,
  IconPlayerPlay,
  IconSend,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "../../../components/Button";
import { Segmented } from "../../../components/Segmented";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../../components/ui/dropdown-menu";
import { Switch } from "../../../components/ui/switch";
import { formatCount } from "../../../lib/format";
import { CardShell, EmptySection, Section, statusDot } from "./CardShell";
import { PROFILE_LABEL, type Scenario } from "./fixtures";

const PROFILES = [
  { key: "trekking", label: "Trekking" },
  { key: "fastbike", label: "Road" },
  { key: "gravel", label: "Gravel" },
] as const;

const DELIVERY_TONE: Record<Scenario["delivery"]["state"], string> = {
  none: "--ink-2",
  current: "--good",
  pending: "--hold",
  failed: "--alert",
};

function deliveryLine(scenario: Scenario): string {
  const { state, sent, total } = scenario.delivery;
  if (state === "none") {
    return "Not published";
  }
  if (state === "failed") {
    return `Push failed · sent to ${sent} of ${formatCount(total, "rider")}`;
  }
  if (state === "pending") {
    return "Sending…";
  }
  return `Sent to ${sent} of ${formatCount(total, "rider")}`;
}

export function VariantA({ scenario }: { scenario: Scenario }) {
  const waypointCount = scenario.waypoints.length;

  return (
    <CardShell
      title={scenario.planName === "" ? "Plan a route" : scenario.planName}
      aside={
        <span className="relative inline-flex">
          <IconSend size={16} stroke={1.8} className="text-[var(--ink-2)]" />
          {scenario.delivery.state === "none" ? null : (
            <span className="absolute -top-0.5 -right-0.5">
              {statusDot(
                DELIVERY_TONE[scenario.delivery.state],
                scenario.delivery.state === "pending",
              )}
            </span>
          )}
        </span>
      }
      footer={
        <div className="flex flex-col gap-2">
          <div className="flex gap-2">
            <Button className="flex-1" disabled={scenario.saving}>
              Save
            </Button>
            {scenario.published ? (
              <DropdownMenu>
                <DropdownMenuTrigger render={<Button variant="outline" />}>
                  Published ✓
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem>Unpublish</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <Button variant="outline" disabled={scenario.saving || waypointCount < 2}>
                Publish
              </Button>
            )}
          </div>
          <p
            className="truncate text-xs"
            style={{ color: `var(${DELIVERY_TONE[scenario.delivery.state]})` }}
          >
            {deliveryLine(scenario)}
          </p>
          {scenario.saveError ? (
            <p className="text-[var(--alert)] text-xs">{scenario.saveError}</p>
          ) : null}
        </div>
      }
    >
      <Section
        title="Route options"
        summary={`${PROFILE_LABEL[scenario.profile]} · Cues ${scenario.cues ? `on (${scenario.turnCount ?? 0})` : "off"}`}
        defaultOpen={false}
      >
        <div className="flex flex-col gap-3 pt-1">
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
                Adds turn instructions to the course on riders' devices.
                {scenario.turnCount === undefined
                  ? ""
                  : ` ${formatCount(scenario.turnCount, "turn")}.`}
              </span>
            </span>
          </label>
        </div>
      </Section>

      {waypointCount === 0 ? (
        <EmptySection>Click the map to place a waypoint.</EmptySection>
      ) : (
        <Section title="Waypoints" summary={formatCount(waypointCount, "stop")} defaultOpen>
          <ol className="flex flex-col gap-1 pt-1">
            {scenario.waypoints.map((waypoint, index) => (
              <li
                key={waypoint.id}
                className="group flex items-center gap-2 rounded-lg px-1.5 py-1.5 text-sm hover:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]"
              >
                <span
                  aria-hidden="true"
                  className={`grid size-6 shrink-0 place-items-center rounded-[8px] font-semibold text-white text-xs ${
                    waypoint.terminal ? "bg-[var(--accent)]" : "bg-[var(--ink-2)]"
                  }`}
                >
                  {index === 0 ? (
                    <IconPlayerPlay aria-hidden="true" size={14} stroke={3} />
                  ) : index === waypointCount - 1 ? (
                    <IconFlagCheck aria-hidden="true" size={15} stroke={2.5} />
                  ) : (
                    index + 1
                  )}
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="flex items-center gap-1.5 truncate">
                    {waypoint.name}
                    {waypoint.straight ? (
                      <span className="shrink-0 rounded-full bg-[color-mix(in_oklab,var(--ink-2)_14%,transparent)] px-1.5 py-0.5 font-medium text-[10px] text-[var(--ink-2)] uppercase tracking-wide">
                        Straight
                      </span>
                    ) : null}
                  </span>
                  <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                    {waypoint.stop}
                    {waypoint.meta ? ` · ${waypoint.meta}` : ""}
                  </span>
                </span>
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
                        icon={<IconDots size={15} />}
                        aria-label={`Actions for ${waypoint.name}`}
                      />
                    }
                  />
                  <DropdownMenuContent align="end">
                    {index === 0 ? null : (
                      <DropdownMenuItem>
                        {waypoint.straight ? "Route normally" : "Straight line to here"}
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem variant="destructive">Delete</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </li>
            ))}
          </ol>
        </Section>
      )}

      {scenario.avoid.length === 0 ? (
        <EmptySection>No avoided areas.</EmptySection>
      ) : (
        <Section
          title="Avoided areas"
          summary={formatCount(scenario.avoid.length, "area")}
          defaultOpen
        >
          <ul className="flex flex-col gap-1.5 pt-1">
            {scenario.avoid.map((area) => (
              <li
                key={area.id}
                className="flex items-center gap-2 rounded-lg bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] px-3 py-2 text-sm"
              >
                <IconBan
                  aria-hidden="true"
                  size={14}
                  stroke={1.8}
                  className="shrink-0 text-[var(--alert)]"
                />
                <span className="min-w-0 flex-1 truncate text-[var(--ink-2)] text-xs">
                  {area.place} · {area.radius}
                </span>
                <Button
                  variant="ghost"
                  icon={<IconTrash size={15} />}
                  aria-label={`Delete ${area.place}`}
                />
              </li>
            ))}
          </ul>
        </Section>
      )}
    </CardShell>
  );
}
