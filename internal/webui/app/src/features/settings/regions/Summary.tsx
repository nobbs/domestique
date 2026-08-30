/**
 * What a selection costs and what is wrong with it, said under the picker.
 *
 * Only Germany's extracts are measured, so this has to be able to say "some of
 * this is unpriced" without either hiding the gap or reporting a four-gigabyte
 * download as nothing.
 */

import { Badge } from "@/components/ui/badge";
import { cost, formatBytes, redundant, unknown } from "./model";

export function Summary({ value }: { value: readonly string[] }) {
  const doubled = redundant(value);
  const unrecognised = unknown(value);
  const { transfer, peakStaging, unmeasured, measured } = cost(value);

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
            {measured > 0 ? (
              <>
                {" · "}
                {unmeasured > 0 ? "at least " : ""}
                {formatBytes(transfer)} downloaded per rebuild
                {" · "}
                {unmeasured > 0 ? "at least " : ""}
                {formatBytes(peakStaging)} staged on disk at once
              </>
            ) : null}
          </>
        )}
      </p>
      {unmeasured > 0 ? (
        <p className="text-[var(--ink-2)]">
          {unmeasured === 1 ? "One selected region has" : `${unmeasured} selected regions have`} no
          published size, so{" "}
          {measured > 0 ? "the totals above are a floor" : "the cost is not known here"}. Sizes are
          collected for Germany.
        </p>
      ) : null}
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
 * A region with no published size renders nothing at all. Most of the catalogue
 * is unmeasured, so a badge saying so would be on hundreds of rows and would
 * read as a fault rather than as an absence.
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
