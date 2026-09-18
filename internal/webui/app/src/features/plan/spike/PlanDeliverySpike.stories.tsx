import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconAlertTriangle,
  IconCheck,
  IconChevronDown,
  IconRefresh,
  IconRoute,
  IconSend,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button, ButtonLink } from "../../../components/Button";
import { InsetList, InsetRow, type RowTone } from "../../../components/InsetList";
import { Panel } from "../../../components/PanelHeading";
import { Badge } from "../../../components/ui/badge";
import { Input } from "../../../components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "../../../components/ui/popover";
import { Spinner } from "../../../components/ui/spinner";
import { formatCount, formatReadTime } from "../../../lib/format";
import { StoryProviders } from "../../../storybook/fixtures";
import {
  type DeliveryScenario,
  type DeliveryState,
  isTrueDraft,
  type PlanTargetDelivery,
  SCENARIOS,
} from "./data";

const STATE_TONE: Record<DeliveryState, RowTone> = {
  current: "good",
  pending: "quiet",
  outdated: "hold",
  failed: "alert",
  absent: "quiet",
};

const TONE_VAR: Record<RowTone, string> = {
  good: "--good",
  hold: "--hold",
  alert: "--alert",
  quiet: "--ink-2",
};

// Sentence-case, matching the wording lib/syncGuidance uses for the Wahoo half of a sync.
const FAILURE_LABEL: Record<NonNullable<PlanTargetDelivery["failure"]>, string> = {
  unauthorized: "Must reconnect Wahoo",
  quota: "Wahoo's daily quota is used up",
  course: "Route could not be encoded",
  state: "Stored state error",
  target: "Wahoo operation failed",
};

function riderLabel(target: PlanTargetDelivery): string {
  if (target.ownerNickname) {
    return target.own ? `${target.ownerNickname} (you)` : target.ownerNickname;
  }
  return target.own ? "You" : "Unnamed rider";
}

/** The short word a row's mark carries, for the trigger and the row's own tooltip. */
function stateLabel(target: PlanTargetDelivery): string {
  if (target.state === "pending") {
    return target.removing ? "Removing…" : "Sending…";
  }
  if (target.state === "absent" && target.deliveredAt) {
    return "Removed";
  }
  return {
    current: "On Wahoo",
    pending: "Sending…",
    outdated: "Older revision",
    failed: "Push failed",
    absent: "Not on Wahoo",
  }[target.state];
}

/** The row's own line: what happened, and when — the popover's reason to exist. */
function rowSecondary(target: PlanTargetDelivery): string {
  if (target.state === "failed") {
    return target.failure ? FAILURE_LABEL[target.failure] : "Push failed";
  }
  if (target.state === "pending") {
    return target.removing ? "Removing…" : "Sending…";
  }
  if (target.state === "current") {
    return `Delivered ${formatReadTime(target.deliveredAt)}`;
  }
  if (target.state === "outdated") {
    return `Older revision · delivered ${formatReadTime(target.deliveredAt)}`;
  }
  return target.deliveredAt ? `Removed ${formatReadTime(target.deliveredAt)}` : "Not on Wahoo";
}

/** What the header line under "On Wahoo" says the plan's delivery is doing. */
function summaryLine(scenario: DeliveryScenario): string {
  const total = scenario.targets.length;
  const removing = scenario.targets.some((target) => target.state === "pending" && target.removing);
  if (removing) {
    return `Removing from ${formatCount(total, "rider")}`;
  }
  const current = scenario.targets.filter((target) => target.state === "current").length;
  return `Sent to ${current} of ${formatCount(total, "rider")}`;
}

/** good/alert/hold read the way `InsetRow` already reads them; `null` draws no dot at all. */
function aggregateTone(scenario: DeliveryScenario): RowTone | null {
  if (isTrueDraft(scenario)) {
    return null;
  }
  if (scenario.targets.some((target) => target.state === "failed")) {
    return "alert";
  }
  if (scenario.targets.some((target) => target.state === "pending")) {
    return "hold";
  }
  if (scenario.targets.every((target) => target.state === "current")) {
    return "good";
  }
  return "hold";
}

function labelFor(scenario: DeliveryScenario): string {
  if (isTrueDraft(scenario)) {
    return "Not published";
  }
  const failed = scenario.targets.filter((target) => target.state === "failed").length;
  if (failed > 0) {
    return `${failed} failed`;
  }
  if (scenario.targets.some((target) => target.state === "pending" && target.removing)) {
    return "Removing…";
  }
  if (scenario.targets.some((target) => target.state === "pending")) {
    return "Sending…";
  }
  return "On Wahoo";
}

function TriggerGlyph({ scenario }: { scenario: DeliveryScenario }) {
  if (scenario.targets.some((target) => target.state === "failed")) {
    return <IconAlertTriangle size={14} stroke={1.8} />;
  }
  if (scenario.targets.some((target) => target.state === "pending")) {
    return <Spinner className="size-3.5" aria-hidden="true" />;
  }
  if (isTrueDraft(scenario)) {
    return <IconSend size={14} stroke={1.8} />;
  }
  return <IconCheck size={14} stroke={1.8} />;
}

type TriggerVariant = "icon" | "pill" | "label";

/** The one thing all three variants disagree about: what sits in the header. */
function Trigger({ scenario, variant }: { scenario: DeliveryScenario; variant: TriggerVariant }) {
  const tone = aggregateTone(scenario);
  const pending = scenario.targets.some((target) => target.state === "pending");

  if (variant === "icon") {
    return (
      <span className="relative inline-flex">
        <PopoverTrigger
          render={<Button variant="ghost" icon={<IconSend size={16} stroke={1.8} />} />}
          aria-label="Wahoo delivery status"
        />
        {tone ? (
          <span
            aria-hidden="true"
            className={`pointer-events-none absolute top-0.5 right-0.5 size-2 rounded-full ring-2 ring-[var(--panel)] ${pending ? "animate-pulse" : ""}`}
            style={{ background: `var(${TONE_VAR[tone]})` }}
          />
        ) : null}
      </span>
    );
  }

  if (variant === "pill") {
    const current = scenario.targets.filter((target) => target.state === "current").length;
    const failed = scenario.targets.some((target) => target.state === "failed");
    const label = isTrueDraft(scenario)
      ? "—"
      : `${current}/${scenario.targets.length}${failed ? " !" : pending ? " ⟳" : ""}`;

    return (
      <PopoverTrigger
        render={
          <Badge
            variant={tone === "alert" ? "destructive" : tone === "good" ? "outline" : "secondary"}
            className={`cursor-pointer font-mono tabular-nums ${pending ? "animate-pulse" : ""}`}
          />
        }
        aria-label="Wahoo delivery status"
      >
        {label}
      </PopoverTrigger>
    );
  }

  return (
    <PopoverTrigger
      render={<Button variant="ghost" icon={<TriggerGlyph scenario={scenario} />} />}
      aria-label="Wahoo delivery status"
    >
      {labelFor(scenario)}
    </PopoverTrigger>
  );
}

/**
 * One popover, shared by every trigger variant: a header naming the summary,
 * then one generously-spaced row per rider — mark, name, and what happened,
 * in words. A failed row is the only one that offers to try again.
 */
function DeliveryPopoverContent({ scenario }: { scenario: DeliveryScenario }) {
  return (
    <PopoverContent align="start" aria-label="Wahoo delivery status" className="w-80 gap-3 p-3">
      <PopoverHeader>
        <PopoverTitle>On Wahoo</PopoverTitle>
        <PopoverDescription>{summaryLine(scenario)}</PopoverDescription>
      </PopoverHeader>
      {isTrueDraft(scenario) ? (
        <p className="text-[var(--ink-2)] text-sm">
          Publishing sends this plan to every connected rider's Wahoo account.
        </p>
      ) : (
        <InsetList>
          {scenario.targets.map((target) => (
            <InsetRow
              key={target.id}
              tone={STATE_TONE[target.state]}
              toneLabel={stateLabel(target)}
              title={riderLabel(target)}
              detail={rowSecondary(target)}
              actions={
                target.state === "failed" ? (
                  <Button
                    variant="ghost"
                    icon={<IconRefresh size={14} />}
                    aria-label={`Retry ${riderLabel(target)}`}
                  />
                ) : target.state === "pending" ? (
                  <Spinner aria-label={stateLabel(target)} />
                ) : null
              }
            />
          ))}
        </InsetList>
      )}
    </PopoverContent>
  );
}

function DeliveryWidget({
  scenario,
  variant,
  defaultOpen,
}: {
  scenario: DeliveryScenario;
  variant: TriggerVariant;
  defaultOpen?: boolean;
}) {
  return (
    <Popover defaultOpen={defaultOpen}>
      <Trigger scenario={scenario} variant={variant} />
      <DeliveryPopoverContent scenario={scenario} />
    </Popover>
  );
}

/**
 * A spike copy of the planner panel's header row — the real one lives in
 * `PlannerSidebar` and is not touched. Same icon, title field and "Plans" /
 * "New" actions; the delivery trigger joins them at the end.
 */
function SpikeHeaderCard({ trigger }: { trigger: ReactNode }) {
  return (
    <Panel
      className="max-w-sm"
      icon={<IconRoute size={18} stroke={1.8} />}
      title={
        <Input
          aria-label="Plan name"
          className="min-w-0 border-transparent border-b-2 bg-transparent px-0 font-semibold text-base shadow-none focus-visible:border-[var(--accent)] focus-visible:ring-0"
          value="Saturday gravel"
          readOnly
        />
      }
      aside={
        <div className="flex items-center gap-1">
          <Button variant="ghost" icon={<IconChevronDown size={16} stroke={2} />} disabled>
            Plans
          </Button>
          <ButtonLink variant="ghost" to="/plan">
            New
          </ButtonLink>
          {trigger}
        </div>
      }
    >
      {null}
    </Panel>
  );
}

const DEFAULT_SCENARIO = SCENARIOS[0] as DeliveryScenario;

function findScenario(key: string): DeliveryScenario {
  return SCENARIOS.find((scenario) => scenario.key === key) ?? DEFAULT_SCENARIO;
}

/** The header, in place, with the popover open so a screenshot shows it. */
function InContext({ scenarioKey, variant }: { scenarioKey: string; variant: TriggerVariant }) {
  const scenario = findScenario(scenarioKey);

  return (
    <SpikeHeaderCard
      key={scenario.key}
      trigger={<DeliveryWidget scenario={scenario} variant={variant} defaultOpen />}
    />
  );
}

/** Every scenario's trigger, closed, side by side — for comparing the mark alone. */
function AllTriggers({ variant }: { variant: TriggerVariant }) {
  return (
    <div className="flex flex-wrap items-end gap-6">
      {SCENARIOS.map((scenario) => (
        <div key={scenario.key} className="flex flex-col items-center gap-1.5">
          <span className="text-[var(--ink-2)] text-xs">{scenario.label}</span>
          <Popover>
            <Trigger scenario={scenario} variant={variant} />
            <DeliveryPopoverContent scenario={scenario} />
          </Popover>
        </div>
      ))}
    </div>
  );
}

interface Args {
  scenarioKey: string;
}

const meta = {
  title: "Spikes/Plan Delivery",
  args: { scenarioKey: "failures" },
  argTypes: {
    scenarioKey: {
      control: "select",
      options: SCENARIOS.map((scenario) => scenario.key),
    },
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="p-4">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<Args>;

export default meta;
type Story = StoryObj<typeof meta>;

export const IconButton: Story = {
  render: (args) => <InContext scenarioKey={args.scenarioKey} variant="icon" />,
};
export const IconButtonAllScenarios: Story = {
  render: () => <AllTriggers variant="icon" />,
};

export const TextPill: Story = {
  render: (args) => <InContext scenarioKey={args.scenarioKey} variant="pill" />,
};
export const TextPillAllScenarios: Story = {
  render: () => <AllTriggers variant="pill" />,
};

export const IconLabel: Story = {
  render: (args) => <InContext scenarioKey={args.scenarioKey} variant="label" />,
};
export const IconLabelAllScenarios: Story = {
  render: () => <AllTriggers variant="label" />,
};
