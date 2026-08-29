/**
 * **D — Columns.** The sideways card, labelling each class with its share.
 *
 * A percentage is what a legend has always carried, and it is the figure that
 * lets two routes be compared: "a tenth gravel" means the same thing on a
 * forty kilometre ride and on a two hundred kilometre one.
 *
 * It is also, on this card, the one thing the bar beside it is already
 * saying — which is the objection **E** is drawn to test. See `SidewaysCard`
 * for the arrangement both share.
 */

import { SidewaysCard } from "./SidewaysCard";
import type { CardProps } from "./shared";
import { formatShare } from "./shared";

export function ColumnsCard(props: CardProps) {
  return <SidewaysCard {...props} figure={(entry) => formatShare(entry.share)} />;
}
