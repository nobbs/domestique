/**
 * Four takes on the ride page's sensors card, at the rail's 22rem.
 *
 * Storybook only. Every variant reads the real grouped figures of one synthetic ride.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconActivity } from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { Activity } from "../../../api/types";
import { PanelHeading, PanelMark } from "../../../components/PanelHeading";
import {
  buildGroups,
  type Group,
  groupedSections,
  type Scale,
  TrainingLoad,
} from "../TrainingLoad";
import { ELAPSED_SECONDS, METRICS, MOVING_SECONDS, TOTAL_METRES } from "./data";

const RIDE: Activity = {
  id: "42",
  startedAt: "2026-09-12T09:04:00Z",
  distanceMetres: TOTAL_METRES,
  movingSeconds: MOVING_SECONDS,
  elapsedSeconds: ELAPSED_SECONDS,
  ascentMetres: 836,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
  metrics: {
    ...METRICS,
    zoneSeconds: [0, 0, 0, 0, 0],
    maxSpeedKmh: 58.3,
    maxCadenceRpm: 112,
    maxPowerWatts: 740,
  },
};

const GROUPS: Group[] = groupedSections(buildGroups(RIDE, RIDE.metrics));
const shown = (group: Group) => group.figures.filter((figure) => figure.value !== undefined);
const value = (figure: Scale) => figure.value?.toFixed(figure.decimals ?? 0) ?? "—";
const unit = (figure: Scale) => (figure.max !== undefined ? figure.scale : "");

function Card({ children }: { children: ReactNode }) {
  return (
    <section className="flex w-[22rem] flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
      <PanelHeading icon={<IconActivity size={18} stroke={1.8} />} title="Sensors" />
      {children}
    </section>
  );
}

/** A · Today's tinted tiles. */
function Today() {
  return (
    <div className="w-[22rem]">
      <TrainingLoad ride={RIDE} />
    </div>
  );
}

/** B · Ledger: a hairline table a group, muted group heading, value and peak right-aligned. */
function Ledger() {
  return (
    <Card>
      {GROUPS.map((group) => (
        <div key={group.title} className="flex flex-col">
          <span className="pb-1 text-[var(--ink-2)] text-xs">{group.title}</span>
          <dl className="flex flex-col divide-y divide-[var(--rule)]">
            {shown(group).map((figure) => {
              const Icon = figure.icon;
              return (
                <div key={figure.label} className="flex items-center gap-2 py-1.5 text-sm">
                  <span style={{ color: figure.colour }}>
                    <Icon size={15} stroke={1.8} aria-hidden="true" />
                  </span>
                  <dt className="flex-1">{figure.label}</dt>
                  <dd className="font-semibold tabular-nums">
                    {value(figure)}
                    <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">
                      {unit(figure) || figure.scale}
                    </span>
                  </dd>
                  <dd className="w-14 text-right text-[var(--ink-2)] text-xs tabular-nums">
                    {figure.max !== undefined
                      ? `max ${figure.max.toFixed(figure.decimals ?? 0)}`
                      : ""}
                  </dd>
                </div>
              );
            })}
          </dl>
        </div>
      ))}
    </Card>
  );
}

/** C · Marked rows: the dark mark of the page's headings, small, one row a figure, groups split by a rule. */
function MarkedRows() {
  return (
    <Card>
      <div className="flex flex-col divide-y divide-[var(--rule)]">
        {GROUPS.map((group) => (
          <div
            key={group.title}
            className="grid grid-cols-2 gap-x-3 gap-y-3 py-3 first:pt-0 last:pb-0"
          >
            {shown(group).map((figure) => {
              const Icon = figure.icon;
              return (
                <div key={figure.label} className="flex items-center gap-2.5">
                  <span className="scale-[0.8]">
                    <PanelMark>
                      <Icon size={18} stroke={1.8} aria-hidden="true" />
                    </PanelMark>
                  </span>
                  <div className="flex min-w-0 flex-col">
                    <span className="truncate text-[var(--ink-2)] text-xs">{figure.label}</span>
                    <span className="font-semibold tabular-nums">
                      {value(figure)}
                      <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">
                        {unit(figure) || figure.scale}
                      </span>
                    </span>
                    {figure.max !== undefined ? (
                      <span className="text-[var(--ink-2)] text-xs tabular-nums">
                        max {figure.max.toFixed(figure.decimals ?? 0)}
                      </span>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </Card>
  );
}

/** D · Quiet tiles: today's grid, the tint dropped to a neutral wash, colour kept to the icon and a top rule. */
function QuietTiles() {
  return (
    <Card>
      <div className="flex flex-col gap-2">
        {GROUPS.map((group) => (
          <div
            key={group.title}
            className={
              shown(group).length === 3 ? "grid grid-cols-3 gap-2" : "grid grid-cols-2 gap-2"
            }
          >
            {shown(group).map((figure) => {
              const Icon = figure.icon;
              return (
                <div
                  key={figure.label}
                  className="grid content-start gap-0.5 rounded-lg border-t-2 bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] p-2.5"
                  style={{ borderTopColor: figure.colour }}
                >
                  <span className="flex items-center gap-1 text-[var(--ink-2)] text-xs">
                    <span style={{ color: figure.colour }}>
                      <Icon size={13} stroke={1.8} aria-hidden="true" />
                    </span>
                    {figure.label}
                  </span>
                  <span className="font-semibold text-lg leading-tight tabular-nums">
                    {value(figure)}
                    <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">
                      {unit(figure)}
                    </span>
                  </span>
                  <span className="text-[var(--ink-2)] text-xs tabular-nums">
                    {figure.max !== undefined
                      ? `max ${figure.max.toFixed(figure.decimals ?? 0)}`
                      : figure.scale}
                  </span>
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </Card>
  );
}

const meta = {
  title: "Spikes/Ride Sensors Card",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllVariants: Story = {
  render: () => (
    <div className="flex min-h-dvh flex-wrap items-start gap-8 bg-[var(--base)] p-8 text-[var(--ink)]">
      {[
        ["A · Today", <Today key="a" />],
        ["B · Ledger", <Ledger key="b" />],
        ["C · Marked rows", <MarkedRows key="c" />],
        ["D · Quiet tiles", <QuietTiles key="d" />],
      ].map(([name, node]) => (
        <section key={name as string} className="flex flex-col gap-2">
          <span className="text-[var(--ink-2)] text-xs">{name}</span>
          {node}
        </section>
      ))}
    </div>
  ),
};
