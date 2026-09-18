/**
 * Three structures for Admin: tasks folded in, settings grouped by area, and
 * the same section rail as Account.
 *
 * Storybook only. Every card is the real one over the story fixtures.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconBell,
  IconListCheck,
  IconMap,
  IconPlugConnected,
  IconServer,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { StoryProviders, settings } from "../../../storybook/fixtures";
import { Group, SectionRail, SpikePage, type SpikeSection } from "../../account/spike/layouts";
import { Missing, ServiceSettings } from "../ServiceSettings";
import { TaskRunFeed } from "../tasks/TaskRunFeed";
import { TaskTable } from "../tasks/TaskTable";

const icon = (Glyph: typeof IconServer) => <Glyph size={16} stroke={1.8} />;
const CARD = "flex flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]";

function TaskCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className={CARD}>
      <h3 className="font-semibold text-base">{title}</h3>
      {children}
    </section>
  );
}

const Tasks = () => (
  <>
    <TaskCard title="Background tasks">
      <TaskTable />
    </TaskCard>
    <TaskCard title="What has happened">
      <TaskRunFeed />
    </TaskCard>
  </>
);

const GROUPS = {
  service: (
    <>
      <Missing missing={settings.missing} />
      <ServiceSettings group="service" />
    </>
  ),
  integrations: <ServiceSettings group="integrations" />,
  alerting: <ServiceSettings group="alerting" />,
  map: <ServiceSettings group="map" />,
};

const SECTIONS: SpikeSection[] = [
  { key: "service", label: "Service", icon: icon(IconServer), content: GROUPS.service },
  {
    key: "integrations",
    label: "Integrations",
    icon: icon(IconPlugConnected),
    content: GROUPS.integrations,
  },
  { key: "alerting", label: "Alerting", icon: icon(IconBell), content: GROUPS.alerting },
  { key: "map", label: "Map", icon: icon(IconMap), content: GROUPS.map },
  { key: "tasks", label: "Tasks", icon: icon(IconListCheck), content: <Tasks /> },
];

const page = (render: () => React.JSX.Element): Story => ({
  render: () => (
    <StoryProviders>
      <SpikePage title="Admin">{render()}</SpikePage>
    </StoryProviders>
  ),
});

const meta = {
  title: "Spikes/Admin Layout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** 1 · Tasks folded in: today's long settings page with the tasks appended, one page. */
export const TasksFoldedIn = page(() => (
  <div className="flex max-w-3xl flex-col gap-5">
    <ServiceSettings />
    <Group title="Tasks">
      <Tasks />
    </Group>
  </div>
));

/** 2 · Grouped by area: one long page split under Service, Integrations, Alerting, Map, Tasks. */
export const GroupedByArea = page(() => (
  <div className="flex max-w-3xl flex-col gap-6">
    <Group title="Service">{GROUPS.service}</Group>
    <Group title="Integrations">{GROUPS.integrations}</Group>
    <Group title="Alerting">{GROUPS.alerting}</Group>
    <Group title="Map">{GROUPS.map}</Group>
    <Group title="Tasks">
      <Tasks />
    </Group>
  </div>
));

/** 3 · Section rail: the same groups in Account's rail, one shown at a time. */
export const SectionRailLayout = page(() => <SectionRail sections={SECTIONS} />);
