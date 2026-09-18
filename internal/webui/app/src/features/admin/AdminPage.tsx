/**
 * Admin: the service's shared settings, grouped by area, and its background
 * tasks. Each is a tab at `/admin/{section}`; what setup still lacks stays above
 * them, whichever tab is open.
 */

import {
  IconBell,
  IconHistory,
  IconListCheck,
  IconMap,
  IconPlugConnected,
  IconServer,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { settingsQuery } from "../../api/queries";
import { PageShell } from "../../components/Layout";
import { type PageTab, PageTabs } from "../../components/PageTabs";
import { Panel } from "../../components/PanelHeading";
import { Missing, ServiceSettings } from "./ServiceSettings";
import { TaskRunFeed } from "./tasks/TaskRunFeed";
import { TaskTable } from "./tasks/TaskTable";

const icon = (Glyph: typeof IconServer, size = 15) => (
  <Glyph size={size} stroke={1.8} aria-hidden="true" />
);

function TasksTab() {
  return (
    <>
      <Panel icon={icon(IconListCheck, 18)} title="Background tasks">
        <TaskTable />
      </Panel>
      <Panel icon={icon(IconHistory, 18)} title="What has happened">
        <TaskRunFeed />
      </Panel>
    </>
  );
}

const TABS: readonly PageTab[] = [
  {
    key: "service",
    label: "Service",
    icon: icon(IconServer),
    content: <ServiceSettings group="service" />,
  },
  {
    key: "integrations",
    label: "Integrations",
    icon: icon(IconPlugConnected),
    content: <ServiceSettings group="integrations" />,
  },
  {
    key: "alerting",
    label: "Alerting",
    icon: icon(IconBell),
    content: <ServiceSettings group="alerting" />,
  },
  { key: "map", label: "Map", icon: icon(IconMap), content: <ServiceSettings group="map" /> },
  { key: "tasks", label: "Tasks", icon: icon(IconListCheck), content: <TasksTab /> },
];

export function AdminPage() {
  const { data } = useQuery(settingsQuery());

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Admin</h1>
        <Missing missing={data?.missing ?? []} />
        <PageTabs label="Admin sections" base="/admin" tabs={TABS} />
      </div>
    </PageShell>
  );
}
