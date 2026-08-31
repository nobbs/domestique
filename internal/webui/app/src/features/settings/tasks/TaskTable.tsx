/**
 * Every registered background task, in registration order: its cadence, when
 * it is next due, and the controls to switch or run it.
 *
 * A switch governs the schedule only. The Run button beside it runs the task
 * whether the switch is on or off, which is why it never disables itself for a
 * switched-off task: turning the schedule off is a statement about unattended
 * runs, not a lock.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useRunTask, useRunTaskArgument, useSetTaskSchedule } from "../../../api/generated";
import { statusQuery, taskRunsQueryPrefix, tasksQuery } from "../../../api/queries";
import { TASKS } from "../../../api/tasks";
import type { Task } from "../../../api/types";
import { Button } from "../../../components/Button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "../../../components/ui/alert-dialog";
import { Badge } from "../../../components/ui/badge";
import { Input } from "../../../components/ui/input";
import { RadioGroup, RadioGroupItem } from "../../../components/ui/radio-group";
import { Skeleton } from "../../../components/ui/skeleton";
import { Spinner } from "../../../components/ui/spinner";
import { Switch } from "../../../components/ui/switch";
import { formatCadence, formatTimestamp } from "../../../lib/format";

/** A task's cadence, in the words its own switch would mean, or "On demand" for one nothing schedules. */
function cadenceLabel(task: Task): string {
  return task.scheduled ? formatCadence(task.intervalSeconds) : "On demand";
}

/** Absent both while the task's own scheduled run is under way and for a task nothing schedules. */
function nextDueLabel(task: Task): string {
  return task.nextRunAt ? formatTimestamp(task.nextRunAt) : "—";
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function TaskTable() {
  const queryClient = useQueryClient();
  const tasks = useQuery(tasksQuery());
  const status = useQuery(statusQuery());
  const invalidateTasks = () => queryClient.invalidateQueries({ queryKey: tasksQuery().queryKey });
  // Settled, unlike the list: a refusal starts nothing, so what is running is
  // unchanged, but it is recorded, and the history is where its reason is read.
  const invalidateHistory = () =>
    queryClient.invalidateQueries({ queryKey: taskRunsQueryPrefix() });

  const schedule = useSetTaskSchedule({ mutation: { onSuccess: invalidateTasks } });
  const run = useRunTask({
    mutation: { onSuccess: invalidateTasks, onSettled: invalidateHistory },
  });

  // sync:clear is destructive over exactly one slot and a run with none named
  // does nothing (internal/sync/service.go skips a slot it does not
  // recognise), so its dialog holds which slot as well as the confirmation
  // typed against it. Held here rather than per row: only one row offers it.
  const [clearOpen, setClearOpen] = useState(false);
  const [clearSlot, setClearSlot] = useState<string | null>(null);
  const [clearConfirmation, setClearConfirmation] = useState("");
  const clear = useRunTaskArgument({
    mutation: {
      onSuccess: () => {
        setClearOpen(false);
        setClearSlot(null);
        setClearConfirmation("");

        return invalidateTasks();
      },
      onSettled: invalidateHistory,
    },
  });
  const slots = status.data?.targets.map((target) => target.id) ?? [];

  if (tasks.isPending) {
    return <Skeleton className="h-40 w-full" role="status" aria-label="Loading tasks" />;
  }
  if (tasks.isError) {
    return <p className="text-sm text-[var(--alert)]">The service did not say what it runs.</p>;
  }

  function runControl(task: Task) {
    if (task.name !== TASKS.syncClear) {
      return (
        <Button
          variant="outline"
          disabled={run.isPending}
          onClick={() => run.mutate({ name: task.name })}
          aria-label={`Run now: ${task.name}`}
        >
          {run.isPending ? <Spinner aria-label={`Running ${task.name}`} /> : null}
          Run now
        </Button>
      );
    }

    return (
      <AlertDialog
        open={clearOpen}
        onOpenChange={(next) => {
          setClearOpen(next);
          if (!next) {
            setClearSlot(null);
            setClearConfirmation("");
          }
        }}
      >
        <AlertDialogTrigger
          render={<Button variant="destructive" disabled={clear.isPending || slots.length === 0} />}
          aria-label={`Run now: ${task.name}`}
        >
          {clear.isPending ? <Spinner aria-label="Deleting" /> : null}
          Run…
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Domestique routes from a target?</AlertDialogTitle>
            <AlertDialogDescription>
              Choose which target. Routes you made yourself are left alone. The next sync puts these
              back, and no other sync starts until this clear finishes.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <RadioGroup
            value={clearSlot ?? ""}
            onValueChange={(value) => {
              setClearSlot(String(value));
              setClearConfirmation("");
            }}
          >
            {slots.map((id) => (
              <label key={id} className="flex items-center gap-2 text-sm">
                <RadioGroupItem value={id} />
                {id}
              </label>
            ))}
          </RadioGroup>
          {clearSlot ? (
            <label className="grid gap-1 text-sm" htmlFor="clear-task-confirmation">
              Type <strong>{clearSlot}</strong> to confirm.
              <Input
                id="clear-task-confirmation"
                value={clearConfirmation}
                onChange={(event) => setClearConfirmation(event.target.value)}
                autoComplete="off"
              />
            </label>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel render={<Button variant="outline" />}>Cancel</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={clearSlot === null || clearConfirmation !== clearSlot || clear.isPending}
              onClick={() =>
                clearSlot && clear.mutate({ name: TASKS.syncClear, argument: clearSlot })
              }
            >
              {clear.isPending ? <Spinner aria-label="Deleting" /> : null}
              Delete them
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    );
  }

  return (
    <>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-[var(--rule)] text-[var(--ink-2)]">
              <th className="py-2 pr-3 font-medium">Task</th>
              <th className="py-2 pr-3 font-medium">Cadence</th>
              <th className="py-2 pr-3 font-medium">Next due</th>
              <th className="py-2 pr-3 font-medium">Scheduled</th>
              <th className="py-2 font-medium">
                <span className="sr-only">Run</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {tasks.data.tasks.map((task) => (
              <tr key={task.name} className="border-b border-[var(--rule)] last:border-0">
                <td className="py-2 pr-3 font-semibold">
                  <span className="flex items-center gap-2">
                    {task.name}
                    {task.running > 0 ? <Badge variant="secondary">Running</Badge> : null}
                  </span>
                </td>
                <td className="py-2 pr-3">{cadenceLabel(task)}</td>
                <td className="py-2 pr-3 text-[var(--ink-2)]">{nextDueLabel(task)}</td>
                <td className="py-2 pr-3">
                  <Switch
                    checked={task.enabled}
                    disabled={schedule.isPending}
                    onCheckedChange={() =>
                      schedule.mutate({ name: task.name, data: { enabled: !task.enabled } })
                    }
                    aria-label={`Scheduled: ${task.name}`}
                  />
                </td>
                <td className="py-2">{runControl(task)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {schedule.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          The schedule was not changed. It is still what it was.
        </p>
      ) : null}
      {run.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {errorMessage(run.error, "That task could not be run.")}
        </p>
      ) : null}
      {clear.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {errorMessage(clear.error, "Those routes could not be deleted.")}
        </p>
      ) : null}
    </>
  );
}
