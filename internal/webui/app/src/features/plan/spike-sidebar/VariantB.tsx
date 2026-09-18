/**
 * Variant B: rows stay clean until hovered/focused; straight legs read as a
 * dashed connector between badges instead of per-row text. One footer button
 * carries both save and publish, steered by a small draft/published toggle.
 */

import { IconBan, IconLine, IconSend, IconTrash } from "@tabler/icons-react";
import { Button } from "../../../components/Button";
import { Segmented } from "../../../components/Segmented";
import { Switch } from "../../../components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../../components/ui/tooltip";
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

function footerLabel(scenario: Scenario, intendPublished: boolean): string {
  if (scenario.saving) {
    return "Saving…";
  }
  return intendPublished ? "Save & send to Wahoo" : "Save";
}

export function VariantB({ scenario }: { scenario: Scenario }) {
  const waypointCount = scenario.waypoints.length;
  // The footer's own toggle only proposes a next state; the scenario's `published` still draws the status.
  const intendPublished = scenario.published;

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
          <div className="flex items-center gap-2">
            <label className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
              <Switch size="sm" checked={intendPublished} onCheckedChange={() => {}} />
              {intendPublished ? "Published" : "Draft"}
            </label>
            <Button
              className="flex-1"
              disabled={scenario.saving || waypointCount < 2}
              variant={intendPublished ? "default" : "outline"}
            >
              {footerLabel(scenario, intendPublished)}
            </Button>
          </div>
          {scenario.delivery.state === "failed" ? (
            <p className="text-[var(--alert)] text-xs">
              Push failed · sent to {scenario.delivery.sent} of{" "}
              {formatCount(scenario.delivery.total, "rider")}
            </p>
          ) : scenario.delivery.state === "current" ? (
            <p className="text-[var(--good)] text-xs">
              Sent to {scenario.delivery.sent} of {formatCount(scenario.delivery.total, "rider")}
            </p>
          ) : null}
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
          <ol className="flex flex-col pt-1">
            {scenario.waypoints.map((waypoint, index) => (
              <li key={waypoint.id} className="flex flex-col">
                {index === 0 ? null : (
                  <div
                    aria-hidden="true"
                    className={`ml-3 h-2.5 w-px border-l-2 ${
                      scenario.waypoints[index - 1]?.straight
                        ? "border-dashed border-[var(--ink-2)]"
                        : "border-transparent"
                    }`}
                  />
                )}
                <div className="group flex items-center gap-2.5 rounded-lg px-1.5 py-1 text-sm">
                  <span
                    aria-hidden="true"
                    className={`grid size-6 shrink-0 place-items-center rounded-full font-semibold text-white text-xs ${
                      waypoint.terminal ? "bg-[var(--accent)]" : "bg-[var(--ink-2)]"
                    }`}
                  >
                    {index === 0 ? "S" : index === waypointCount - 1 ? "F" : index + 1}
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate">{waypoint.name}</span>
                    <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                      {waypoint.stop}
                      {waypoint.meta ? ` · ${waypoint.meta}` : ""}
                    </span>
                  </span>
                  <div className="flex shrink-0 items-center gap-0.5 opacity-0 focus-within:opacity-100 group-hover:opacity-100">
                    {index === 0 ? null : (
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              variant="ghost"
                              icon={<IconLine size={14} />}
                              aria-pressed={Boolean(waypoint.straight)}
                              aria-label={
                                waypoint.straight ? "Route normally" : "Straight line to here"
                              }
                            />
                          }
                        />
                        <TooltipContent>
                          {waypoint.straight ? "Route normally" : "Straight line to here"}
                        </TooltipContent>
                      </Tooltip>
                    )}
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            variant="ghost"
                            icon={<IconTrash size={14} />}
                            aria-label={`Delete ${waypoint.name}`}
                          />
                        }
                      />
                      <TooltipContent>Delete</TooltipContent>
                    </Tooltip>
                  </div>
                </div>
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
          <ul className="flex flex-col gap-1 pt-1">
            {scenario.avoid.map((area) => (
              <li
                key={area.id}
                className="group flex items-center gap-2 rounded-lg px-1.5 py-1.5 text-sm"
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
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant="ghost"
                        icon={<IconTrash size={14} />}
                        aria-label={`Delete ${area.place}`}
                        className="opacity-0 focus-visible:opacity-100 group-hover:opacity-100"
                      />
                    }
                  />
                  <TooltipContent>Delete</TooltipContent>
                </Tooltip>
              </li>
            ))}
          </ul>
        </Section>
      )}
    </CardShell>
  );
}
