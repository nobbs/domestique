/**
 * Tasks, as a settings subpage: the background layer's schedule and manual
 * controls, and what it has been doing.
 */

import { PageShell } from "../../../components/Layout";
import { Card, CardContent, CardHeader, CardTitle } from "../../../components/ui/card";
import { TaskRunFeed } from "./TaskRunFeed";
import { TaskTable } from "./TaskTable";

export function TasksPage() {
  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="text-2xl font-semibold tracking-tight">Tasks</h1>
        <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
          <CardHeader>
            <CardTitle role="heading" aria-level={2}>
              Background tasks
            </CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4">
            <TaskTable />
          </CardContent>
        </Card>
        <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
          <CardHeader>
            <CardTitle role="heading" aria-level={2}>
              What has happened
            </CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4">
            <TaskRunFeed />
          </CardContent>
        </Card>
      </div>
    </PageShell>
  );
}
