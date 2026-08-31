/**
 * What the background tasks have been doing, newest first.
 *
 * Filterable to one task, held in the `task` search param rather than local
 * state, so a filtered view is linkable and survives a reload.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useSearchParams } from "react-router";
import { taskRunsQueryKey, tasksQuery, useTaskRuns } from "../../../api/queries";
import { Button } from "../../../components/Button";
import { Skeleton } from "../../../components/ui/skeleton";
import { Spinner } from "../../../components/ui/spinner";
import { TaskRunRow } from "./TaskRunRow";

/** What names one row apart from the others; see where it is used. */
function runKey(run: { task: string; argument?: string; startedAt: string; reference: string }) {
  return `${run.task}-${run.argument ?? ""}-${run.startedAt}-${run.reference}`;
}

/** The `task` search param, and the names offered to filter it to. */
function TaskFilter({ value, options }: { value: string | undefined; options: string[] }) {
  const [, setParams] = useSearchParams();

  return (
    <label className="flex items-center gap-2 text-sm" htmlFor="task-run-filter">
      Task
      <select
        id="task-run-filter"
        className="h-8 rounded-lg border border-[var(--rule)] bg-transparent px-2 text-sm"
        value={value ?? ""}
        onChange={(event) => {
          const next = event.target.value;
          setParams(
            (current) => {
              const params = new URLSearchParams(current);
              if (next === "") {
                params.delete("task");
              } else {
                params.set("task", next);
              }

              return params;
            },
            { replace: true },
          );
        }}
      >
        <option value="">Every task</option>
        {options.map((name) => (
          <option key={name} value={name}>
            {name}
          </option>
        ))}
      </select>
    </label>
  );
}

/** The body of the "What has happened" card: the task filter, then one row per recorded run. */
export function TaskRunFeed() {
  const queryClient = useQueryClient();
  const [params] = useSearchParams();
  const taskFilter = params.get("task") || undefined;
  const tasks = useQuery(tasksQuery());
  const { data, isPending, isError, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useTaskRuns(taskFilter);

  // A run that finishes writes a row this list does not know about. tasksQuery
  // is already polled while an attempt is in flight, so its return to nothing
  // running is the signal. A change triggers the refresh, not a value: the
  // first return to idle seen is the one the page loaded with.
  const runningNow = tasks.data?.tasks.some((task) => task.running > 0) ?? false;
  const wasRunning = useRef(runningNow);
  useEffect(() => {
    if (wasRunning.current && !runningNow) {
      void queryClient.invalidateQueries({ queryKey: taskRunsQueryKey(taskFilter) });
    }
    wasRunning.current = runningNow;
  }, [runningNow, taskFilter, queryClient]);

  const filter = (
    <TaskFilter value={taskFilter} options={tasks.data?.tasks.map((task) => task.name) ?? []} />
  );

  if (isPending) {
    return (
      <>
        {filter}
        <Skeleton className="h-24 w-full" role="status" aria-label="Loading task history" />
      </>
    );
  }
  if (isError) {
    return (
      <>
        {filter}
        <p className="text-sm text-[var(--alert)]">The service did not say what has happened.</p>
      </>
    );
  }

  const runs = data.pages.flatMap((page) => page.runs);

  return (
    <>
      {filter}
      {runs.length === 0 ? (
        <p className="text-sm text-[var(--ink-2)]">Nothing has run yet.</p>
      ) : (
        <ul className="grid gap-2">
          {runs.map((run) => (
            <TaskRunRow key={runKey(run)} run={run} />
          ))}
        </ul>
      )}
      {hasNextPage ? (
        <Button
          variant="outline"
          disabled={isFetchingNextPage}
          onClick={() => void fetchNextPage()}
        >
          {isFetchingNextPage ? <Spinner aria-label="Loading earlier runs" /> : null}
          Earlier runs
        </Button>
      ) : (
        <p className="text-sm text-[var(--ink-2)]">
          Older runs are pruned as new ones are recorded.
        </p>
      )}
    </>
  );
}
