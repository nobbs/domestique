/**
 * What a selection costs and what is wrong with it, said the same way under
 * every variant so the variants can be compared on their interaction alone.
 */

import { Badge } from "@/components/ui/badge";
import { formatBytes, peakStagingBytes, redundant, transferBytes, unknown } from "./model";

export function Summary({ value }: { value: readonly string[] }) {
  const doubled = redundant(value);
  const unrecognised = unknown(value);

  return (
    <div className="grid gap-2 text-sm">
      <p className="text-[var(--ink-2)]">
        {value.length === 0 ? (
          "No region selected — surface classification is off."
        ) : (
          <>
            <span className="text-[var(--ink)]">
              {value.length} {value.length === 1 ? "region" : "regions"}
            </span>
            {" · "}
            {formatBytes(transferBytes(value))} downloaded per rebuild
            {" · "}
            {formatBytes(peakStagingBytes(value))} staged on disk at once
          </>
        )}
      </p>
      {doubled.length > 0 ? (
        <p className="text-[var(--alert)]">
          {doubled.join(", ")} {doubled.length === 1 ? "is" : "are"} already inside another selected
          region, so {doubled.length === 1 ? "it is" : "they are"} fetched and indexed twice.
        </p>
      ) : null}
      {unrecognised.length > 0 ? (
        <p className="text-[var(--alert)]">
          {unrecognised.join(", ")} {unrecognised.length === 1 ? "is" : "are"} not in the catalogue.
          Kept as typed — a rebuild will fail on {unrecognised.length === 1 ? "it" : "them"} if the
          name is wrong.
        </p>
      ) : null}
    </div>
  );
}

/**
 * One region's size, shown beside its name wherever a variant lists regions.
 *
 * A region with no published size renders nothing at all. Only Germany is
 * measured, so a badge reading "size unknown" would be on four hundred rows and
 * would read as a fault rather than as an absence.
 */
export function Size({ bytes }: { bytes: number | null }) {
  if (bytes === null) {
    return null;
  }

  return (
    <Badge variant="outline" className="tabular-nums">
      {formatBytes(bytes)}
    </Badge>
  );
}
