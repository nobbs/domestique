/**
 * What a language model made of one ride, as the plain text it wrote. Absent
 * until the ride has been analysed, and on a deployment that never had a token.
 */

import type { Activity } from "../../api/types";
import { formatTimestamp } from "../../lib/format";

export function RideAnalysis({ ride }: { ride: Activity | undefined }) {
  const analysis = ride?.analysis;
  if (!analysis) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-2 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Analysis"
    >
      <h2 className="font-medium text-sm">Analysis</h2>
      {/* The model is asked for plain paragraphs; any markup it returns shows as typed. */}
      <p className="whitespace-pre-line text-sm leading-relaxed">{analysis.text}</p>
      <p className="text-[var(--ink-2)] text-xs">
        {analysis.model} · {formatTimestamp(analysis.analysedAt)}
      </p>
    </section>
  );
}
