/**
 * The legend a chart's numbers live in: one row per key, its colour, its name,
 * an optional detail, a value and a share.
 *
 * It is the accessible reading of whatever chart sits beside it, and the
 * second handle on it: pointing at a row names its key, and a key named from
 * elsewhere sets the other rows back.
 */

import { cn } from "@/lib/utils";

export interface LegendRow<K extends string | number> {
  key: K;
  colour: string;
  label: string;
  detail?: string | undefined;
  value: string;
  share?: string | undefined;
}

export interface LegendTableProps<K extends string | number> {
  rows: readonly LegendRow<K>[];
  active?: K | null;
  onActive?: (key: K | null) => void;
  className?: string;
}

export function LegendTable<K extends string | number>({
  rows,
  active = null,
  onActive,
  className,
}: LegendTableProps<K>) {
  return (
    <table
      className={cn("w-full text-sm tabular-nums", className)}
      onMouseLeave={() => onActive?.(null)}
    >
      <tbody>
        {rows.map((row) => (
          <tr
            key={row.key}
            data-active={active === row.key ? "" : undefined}
            className="border-[var(--rule)] border-b transition-opacity duration-150 last:border-0"
            style={{ opacity: active !== null && active !== row.key ? 0.5 : 1 }}
            onMouseEnter={() => onActive?.(row.key)}
          >
            <td className="w-4 py-1.5 pr-2">
              <span
                aria-hidden="true"
                className="inline-block size-2.5 rounded-full align-middle"
                style={{ backgroundColor: row.colour }}
              />
            </td>
            <td className="py-1.5 pr-2">{row.label}</td>
            <td className="py-1.5 pr-2 text-[var(--ink-2)] text-xs">{row.detail}</td>
            <td className="py-1.5 text-right">{row.value}</td>
            <td className="w-10 py-1.5 text-right text-[var(--ink-2)] text-xs">{row.share}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
