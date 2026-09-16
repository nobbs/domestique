/**
 * What a language model made of one ride, as the plain text it wrote. Absent
 * until the ride has been analysed, and on a deployment that never had a token.
 * An admin may ask again about any derived ride while analysis is on.
 */

import { useQuery } from "@tanstack/react-query";
import { useReanalyseActivity } from "../../api/generated";
import { tasksQuery } from "../../api/queries";
import { TASKS } from "../../api/tasks";
import type { Activity } from "../../api/types";
import { Button } from "../../components/Button";
import { formatTimestamp } from "../../lib/format";
import { useEffectiveAdmin } from "../../lib/identity";

export function RideAnalysis({ ride }: { ride: Activity | undefined }) {
  const admin = useEffectiveAdmin();
  const tasks = useQuery({ ...tasksQuery(), enabled: admin });
  const reanalyse = useReanalyseActivity();
  const analysis = ride?.analysis;
  const canAsk =
    admin &&
    ride?.metrics !== undefined &&
    (tasks.data?.tasks.some((task) => task.name === TASKS.activityReanalyse) ?? false);
  if (!ride || (!analysis && !canAsk)) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-2 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]"
      aria-label="Analysis"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-medium text-sm">Analysis</h2>
        {canAsk ? (
          <Button
            variant="outline"
            disabled={reanalyse.isPending || reanalyse.isSuccess}
            onClick={() => reanalyse.mutate({ activityId: ride.id })}
          >
            {analysis ? "Analyse again" : "Analyse"}
          </Button>
        ) : null}
      </div>
      {analysis ? (
        <>
          {/* The model is asked for plain paragraphs; any markup it returns shows as typed. */}
          <p className="whitespace-pre-line text-sm leading-relaxed">{analysis.text}</p>
          <p className="text-[var(--ink-2)] text-xs">
            {analysis.model} · {formatTimestamp(analysis.analysedAt)}
          </p>
        </>
      ) : null}
      {reanalyse.isSuccess ? (
        <p className="text-[var(--ink-2)] text-xs" role="status">
          Asked. Reload the page in a minute to see the new analysis; a failed request leaves this
          one as it is.
        </p>
      ) : null}
      {reanalyse.isError ? (
        <p className="text-[var(--ink-2)] text-xs" role="status">
          The service did not take the request.
        </p>
      ) : null}
    </section>
  );
}
