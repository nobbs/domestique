/**
 * What a language model made of one ride, as the plain text it wrote. Absent
 * until the ride has been analysed, and on a deployment that never had a token.
 * An admin may ask again about any derived ride while analysis is on.
 */

import { IconSparkles } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useReanalyseActivity } from "../../api/generated";
import { tasksQuery } from "../../api/queries";
import { TASKS } from "../../api/tasks";
import type { Activity, ActivityAnalysisDocument } from "../../api/types";
import { Button } from "../../components/Button";
import { PanelHeading } from "../../components/PanelHeading";
import { Badge } from "../../components/ui/badge";
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
        <PanelHeading icon={<IconSparkles size={18} stroke={1.8} />} title="Analysis" />
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
          {analysis.document ? (
            <RideAnalysisDocument analysis={analysis.document} />
          ) : (
            // The model is asked for plain paragraphs; any markup it returns shows as typed.
            <p className="whitespace-pre-line text-sm leading-relaxed">{analysis.text}</p>
          )}
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

function RideAnalysisDocument({ analysis }: { analysis: ActivityAnalysisDocument }) {
  return (
    <>
      <div className="flex items-center gap-2">
        <p className="font-semibold text-sm">{analysis.headline}</p>
        <Badge variant="secondary">{analysis.rideType}</Badge>
      </div>
      <p className="whitespace-pre-line text-sm leading-relaxed">{analysis.summary}</p>
      <p className="text-[var(--ink-2)] text-sm">{analysis.loadEffect}</p>
      <AnalysisList title="Highlights" items={analysis.highlights} />
      <AnalysisList title="Concerns" items={analysis.concerns} />
      <p className="text-sm">
        <span className="font-medium">Next session: </span>
        {analysis.nextSession.advice}
        {analysis.nextSession.suggestedRestDays > 0
          ? ` Suggested rest: ${analysis.nextSession.suggestedRestDays} day(s).`
          : null}
      </p>
      {analysis.dataGaps.length > 0 ? (
        <p className="text-[var(--ink-2)] text-xs">
          Missing figures: {analysis.dataGaps.join(", ")}
        </p>
      ) : null}
    </>
  );
}

function AnalysisList({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) {
    return null;
  }
  return (
    <div className="text-sm">
      <p className="font-medium">{title}</p>
      <ul className="list-disc pl-5">
        {items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </div>
  );
}
