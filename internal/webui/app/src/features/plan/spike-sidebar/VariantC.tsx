/**
 * Variant C: the Route options header doubles as its own quick control — an
 * icon-only profile switch and a cues toggle live on the collapsed summary
 * line, so the common edits never need the section open. Waypoints are a
 * dense table; the footer collapses status and the publish action into one
 * chip beside Save.
 */

import {
  IconBan,
  IconBike,
  IconGripVertical,
  IconRoad,
  IconRoute,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "../../../components/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../../components/ui/dropdown-menu";
import { Switch } from "../../../components/ui/switch";
import { formatCount } from "../../../lib/format";
import { CardShell, EmptySection, Section } from "./CardShell";
import { PROFILE_LABEL, type Scenario } from "./fixtures";

const PROFILE_ICONS = [
  { key: "trekking", icon: <IconBike size={13} stroke={1.8} /> },
  { key: "fastbike", icon: <IconRoad size={13} stroke={1.8} /> },
  { key: "gravel", icon: <IconRoute size={13} stroke={1.8} /> },
] as const;

const CHIP_TONE: Record<Scenario["delivery"]["state"], string> = {
  none: "--ink-2",
  current: "--good",
  pending: "--hold",
  failed: "--alert",
};

function chipLabel(scenario: Scenario): string {
  if (scenario.delivery.state === "pending") {
    return "Sending…";
  }
  if (scenario.delivery.state === "failed") {
    return "Push failed";
  }
  if (scenario.published) {
    return "Published";
  }
  return "Draft";
}

export function VariantC({ scenario }: { scenario: Scenario }) {
  const waypointCount = scenario.waypoints.length;
  const tone = scenario.published ? CHIP_TONE[scenario.delivery.state] : "--ink-2";

  return (
    <CardShell
      title={scenario.planName === "" ? "Plan a route" : scenario.planName}
      footer={
        <div className="flex flex-col gap-2">
          <div className="flex gap-2">
            <Button className="flex-1" disabled={scenario.saving}>
              Save
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="outline"
                    className="gap-1.5"
                    style={{ borderColor: `color-mix(in oklab, var(${tone}) 45%, var(--rule))` }}
                  />
                }
                disabled={waypointCount < 2}
              >
                <span
                  aria-hidden="true"
                  className="size-2 rounded-full"
                  style={{ background: `var(${tone})` }}
                />
                {chipLabel(scenario)}
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {scenario.published ? (
                  <DropdownMenuItem>Unpublish</DropdownMenuItem>
                ) : (
                  <DropdownMenuItem>Publish — sends to Wahoo</DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          {scenario.delivery.state === "failed" ? (
            <p className="text-[var(--alert)] text-xs">
              Sent to {scenario.delivery.sent} of {formatCount(scenario.delivery.total, "rider")}
            </p>
          ) : null}
          {scenario.saveError ? (
            <p className="text-[var(--alert)] text-xs">{scenario.saveError}</p>
          ) : null}
        </div>
      }
    >
      <div className="border-[var(--rule)] border-b py-2">
        <Section
          title="Route options"
          summary={
            <span className="flex items-center gap-2">
              <span className="flex overflow-hidden rounded-md ring-1 ring-[var(--rule)]">
                {PROFILE_ICONS.map((item) => (
                  <span
                    key={item.key}
                    className={`grid size-6 place-items-center ${
                      item.key === scenario.profile
                        ? "bg-[var(--accent)] text-white"
                        : "text-[var(--ink-2)]"
                    }`}
                  >
                    {item.icon}
                  </span>
                ))}
              </span>
              <Switch size="sm" checked={scenario.cues} onCheckedChange={() => {}} />
            </span>
          }
          defaultOpen={false}
        >
          <p className="pt-1 text-[var(--ink-2)] text-xs">
            {PROFILE_LABEL[scenario.profile]} profile.{" "}
            {scenario.cues
              ? `Turn cues on Wahoo, ${formatCount(scenario.turnCount ?? 0, "turn")}.`
              : "Turn cues off."}{" "}
            More route options land here later.
          </p>
        </Section>
      </div>

      {waypointCount === 0 ? (
        <EmptySection>Click the map to place a waypoint.</EmptySection>
      ) : (
        <Section title="Waypoints" summary={formatCount(waypointCount, "stop")} defaultOpen>
          <div className="flex flex-col pt-1">
            {scenario.waypoints.map((waypoint, index) => (
              <div
                key={waypoint.id}
                className="grid grid-cols-[1.25rem_1fr_auto] items-center gap-x-2 border-[var(--rule)] border-b py-1 text-xs last:border-b-0"
              >
                <span
                  aria-hidden="true"
                  className={`text-center font-semibold tabular-nums ${
                    waypoint.terminal ? "text-[var(--accent)]" : "text-[var(--ink-2)]"
                  }`}
                >
                  {index === 0 ? "S" : index === waypointCount - 1 ? "F" : index}
                </span>
                <span className="min-w-0 truncate">
                  {waypoint.straight ? (
                    <span
                      aria-label="Straight leg"
                      title="Straight leg"
                      className="mr-1 text-[var(--ink-2)]"
                    >
                      &#8250;
                    </span>
                  ) : null}
                  {waypoint.name}
                  <span className="ml-1.5 text-[var(--ink-2)] tabular-nums">{waypoint.meta}</span>
                </span>
                <span className="flex shrink-0 items-center gap-0.5">
                  <IconGripVertical
                    aria-hidden="true"
                    size={13}
                    className="cursor-grab text-[var(--ink-2)]"
                  />
                  <Button
                    variant="ghost"
                    icon={<IconTrash size={13} />}
                    aria-label={`Delete ${waypoint.name}`}
                    className="size-6"
                  />
                </span>
              </div>
            ))}
          </div>
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
          <div className="flex flex-col pt-1">
            {scenario.avoid.map((area) => (
              <div
                key={area.id}
                className="grid grid-cols-[1.25rem_1fr_auto] items-center gap-x-2 border-[var(--rule)] border-b py-1 text-xs last:border-b-0"
              >
                <IconBan aria-hidden="true" size={13} className="text-[var(--alert)]" />
                <span className="min-w-0 truncate text-[var(--ink-2)]">
                  {area.place} · {area.radius}
                </span>
                <Button
                  variant="ghost"
                  icon={<IconTrash size={13} />}
                  aria-label={`Delete ${area.place}`}
                  className="size-6"
                />
              </div>
            ))}
          </div>
        </Section>
      )}
    </CardShell>
  );
}
