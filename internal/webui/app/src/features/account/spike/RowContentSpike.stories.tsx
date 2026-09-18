/**
 * Four ways to lay out what an inset row says: sync phases, targets, sync
 * history and task history, each with a healthy row and one that needs someone.
 *
 * Storybook only, over static samples; the inset block is the one already picked.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconAlertTriangle,
  IconCheck,
  IconClockPause,
  IconDots,
  IconHistory,
  IconInfoCircle,
  IconListCheck,
  IconPlayerPlay,
  IconRefresh,
  IconTarget,
} from "@tabler/icons-react";
import { Fragment, type ReactNode } from "react";
import { Button } from "../../../components/Button";
import { Panel } from "../../../components/PanelHeading";
import { Switch } from "../../../components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../../components/ui/tooltip";

type Tone = "good" | "hold" | "alert" | "quiet";

interface Phase {
  label: string;
  meta: string;
  cadence: string;
  on: boolean;
  tone: Tone;
  verdict: string;
  guidance?: { headline: string; remediation: string };
}
interface Target {
  id: string;
  owner?: string;
  current: number;
  total: number;
  tone: Tone;
  verdict: string;
  guidance?: { headline: string; remediation: string };
  connect?: boolean;
}
interface Run {
  when: string;
  title: string;
  meta: string;
  outcome: string;
  tone: Tone;
  reference: string;
}

const PHASES: Phase[] = [
  {
    label: "Read from VeloPlanner",
    meta: "18 Aug 2026, 08:15 · 4 routes",
    cadence: "Hourly",
    on: true,
    tone: "good",
    verdict: "Read",
  },
  {
    label: "Write to Wahoo",
    meta: "18 Aug 2026, 08:30 · 0 created, 2 updated",
    cadence: "Every 6 hours",
    on: false,
    tone: "hold",
    verdict: "Held",
    guidance: {
      headline: "Deletions held.",
      remediation: "More than the per-run maximum would go; confirm on Admin → Service.",
    },
  },
];
const TARGETS: Target[] = [
  { id: "rider-a", current: 4, total: 4, tone: "good", verdict: "In sync" },
  {
    id: "rider-b",
    owner: "github|203061",
    current: 2,
    total: 4,
    tone: "alert",
    verdict: "Not connected",
    guidance: {
      headline: "Wahoo access expired.",
      remediation: "Reconnect to keep its routes current.",
    },
    connect: true,
  },
];
const RUNS: Run[] = [
  {
    when: "18 Aug 2026, 08:30",
    title: "Write to Wahoo",
    meta: "0 created · 2 updated",
    outcome: "Succeeded",
    tone: "good",
    reference: "1a5b3c4d5e6f",
  },
  {
    when: "18 Aug 2026, 08:15",
    title: "Read from VeloPlanner",
    meta: "4 routes",
    outcome: "Failed",
    tone: "alert",
    reference: "9e8d7c6b5a4f",
  },
];
const TASKS: Run[] = [
  {
    when: "18 Aug 2026, 08:15",
    title: "sync:source",
    meta: "Scheduled",
    outcome: "Succeeded",
    tone: "good",
    reference: "2b3c4d5e6f7a",
  },
  {
    when: "17 Aug 2026, 11:00",
    title: "sync:clear · rider-b",
    meta: "Manual · held by another run",
    outcome: "Skipped",
    tone: "hold",
    reference: "8f7e6d5c4b3a",
  },
];

const colour = (tone: Tone) => (tone === "quiet" ? "var(--ink-2)" : `var(--${tone})`);
const tint = (tone: Tone, share = 14) =>
  `color-mix(in oklab, ${colour(tone)} ${share}%, transparent)`;
const ICON: Record<Tone, typeof IconCheck> = {
  good: IconCheck,
  hold: IconClockPause,
  alert: IconAlertTriangle,
  quiet: IconCheck,
};
const mark = (Glyph: typeof IconCheck) => <Glyph size={18} stroke={1.8} />;

const INSET =
  "flex flex-col overflow-hidden rounded-xl bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";
const ROW = "flex gap-3 border-[var(--panel)] border-b-2 px-3.5 py-3 text-sm last:border-b-0";

function Chip({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span
      className="whitespace-nowrap rounded-md px-1.5 py-0.5 font-medium text-xs"
      style={{ background: tint(tone), color: colour(tone) }}
    >
      {children}
    </span>
  );
}

function Info({ text }: { text: string }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            aria-label={text}
            className="-m-1 rounded p-1 text-[var(--ink-2)]"
          />
        }
      >
        <IconInfoCircle size={14} stroke={1.8} aria-hidden="true" />
      </TooltipTrigger>
      <TooltipContent>{text}</TooltipContent>
    </Tooltip>
  );
}

function Progress({ current, total, tone }: { current: number; total: number; tone: Tone }) {
  return (
    <span className="flex w-24 items-center gap-2">
      <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--rule)]">
        <span
          className="block h-full rounded-full"
          style={{ width: `${(current / total) * 100}%`, background: colour(tone) }}
        />
      </span>
      <span className="text-[var(--ink-2)] text-xs tabular-nums">
        {current}/{total}
      </span>
    </span>
  );
}

interface Layout {
  phase: (phase: Phase) => ReactNode;
  target: (target: Target) => ReactNode;
  run: (run: Run) => ReactNode;
}

/** A · Today: a stack of name, detail and guidance; the controls to the right. */
const TODAY: Layout = {
  phase: (p) => (
    <div className={`${ROW} items-start justify-between`}>
      <div className="flex min-w-0 flex-col gap-1">
        <span className="font-semibold">{p.label}</span>
        <span className="text-[var(--ink-2)]">{p.meta}</span>
        {p.guidance ? (
          <span style={{ color: colour(p.tone) }}>
            <strong>{p.guidance.headline}</strong> {p.guidance.remediation}
          </span>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <span className="flex items-center gap-2">
          <Switch checked={p.on} />
          {p.cadence}
        </span>
        <Button variant="outline">Run now</Button>
      </div>
    </div>
  ),
  target: (t) => (
    <div className={`${ROW} items-start justify-between`}>
      <div className="flex min-w-0 flex-col gap-1">
        <span className="font-semibold">{t.id}</span>
        {t.owner ? <span className="text-[var(--ink-2)] text-xs">Owned by {t.owner}</span> : null}
        <span className="text-[var(--ink-2)]">
          {t.current === t.total ? `All ${t.total} routes` : `${t.current} of ${t.total} routes`}
        </span>
        {t.guidance ? (
          <span style={{ color: colour(t.tone) }}>
            <strong>{t.guidance.headline}</strong> {t.guidance.remediation}
          </span>
        ) : null}
      </div>
      <Button variant={t.connect ? "default" : "outline"}>
        {t.connect ? `Connect ${t.id}` : "Reconcile this target"}
      </Button>
    </div>
  ),
  run: (r) => (
    <div className={`${ROW} items-center justify-between`}>
      <div className="flex min-w-0 flex-col gap-1">
        <span className="text-[var(--ink-2)]">{r.when}</span>
        <span className="font-medium">{r.title}</span>
        <span className="text-[var(--ink-2)]">{r.meta}</span>
      </div>
      <span className="flex items-center gap-2">
        <Chip tone={r.tone}>{r.outcome}</Chip>
        <span className="text-[var(--ink-2)] text-xs">{r.reference}</span>
      </span>
    </div>
  ),
};

/** B · Status mark: a tinted glyph leads each row, the name and one muted line beside it, guidance as a tinted note beneath. */
const MARKED: Layout = {
  phase: (p) => (
    <div className={`${ROW} flex-col`}>
      <div className="flex items-center gap-3">
        <StatusMark tone={p.tone} />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="font-semibold">{p.label}</span>
          <span className="truncate text-[var(--ink-2)] text-xs">{p.meta}</span>
        </div>
        <span className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
          {p.cadence}
          <Switch checked={p.on} />
        </span>
        <Button variant="outline" icon={<IconPlayerPlay />} aria-label="Run now" />
      </div>
      {p.guidance ? <Note tone={p.tone} guidance={p.guidance} /> : null}
    </div>
  ),
  target: (t) => (
    <div className={`${ROW} flex-col`}>
      <div className="flex items-center gap-3">
        <StatusMark tone={t.tone} />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="font-semibold">{t.id}</span>
          <span className="truncate text-[var(--ink-2)] text-xs">
            {t.owner ? `${t.owner} · ` : ""}
            {t.current} of {t.total} routes
          </span>
        </div>
        {t.connect ? (
          <Button variant="default">Connect</Button>
        ) : (
          <Button variant="outline" icon={<IconRefresh />} aria-label="Reconcile" />
        )}
      </div>
      {t.guidance ? <Note tone={t.tone} guidance={t.guidance} /> : null}
    </div>
  ),
  run: (r) => (
    <div className={`${ROW} items-center`}>
      <StatusMark tone={r.tone} />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="font-medium">{r.title}</span>
        <span className="truncate text-[var(--ink-2)] text-xs">
          {r.when} · {r.meta}
        </span>
      </div>
      <span className="text-[var(--ink-2)] text-xs">{r.reference}</span>
    </div>
  ),
};

function StatusMark({ tone }: { tone: Tone }) {
  const Glyph = ICON[tone];
  return (
    <span
      aria-hidden="true"
      className="grid size-8 shrink-0 place-items-center rounded-[9px]"
      style={{ background: tint(tone, 18), color: colour(tone) }}
    >
      <Glyph size={16} stroke={2} />
    </span>
  );
}

function Note({
  tone,
  guidance,
}: {
  tone: Tone;
  guidance: { headline: string; remediation: string };
}) {
  return (
    <span
      className="ml-11 rounded-lg px-2.5 py-1.5 text-xs"
      style={{ background: tint(tone, 10), color: colour(tone) }}
    >
      <strong>{guidance.headline}</strong> {guidance.remediation}
    </span>
  );
}

/** C · Figures in columns: name and when on the left; the reading and the verdict in fixed right-hand columns; actions behind ⋯. */
const COLUMNS: Layout = {
  phase: (p) => (
    <div className={`${ROW} items-center`}>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="flex items-center gap-1.5 font-semibold">
          {p.label}
          {p.guidance ? <Info text={`${p.guidance.headline} ${p.guidance.remediation}`} /> : null}
        </span>
        <span className="truncate text-[var(--ink-2)] text-xs">{p.meta}</span>
      </div>
      <span className="w-28 text-[var(--ink-2)] text-xs">{p.cadence}</span>
      <Switch checked={p.on} />
      <span className="w-20 text-right">
        <Chip tone={p.tone}>{p.verdict}</Chip>
      </span>
      <Button variant="ghost" icon={<IconDots />} aria-label="More" />
    </div>
  ),
  target: (t) => (
    <div className={`${ROW} items-center`}>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="flex items-center gap-1.5 font-semibold">
          {t.id}
          {t.guidance ? <Info text={`${t.guidance.headline} ${t.guidance.remediation}`} /> : null}
        </span>
        <span className="truncate text-[var(--ink-2)] text-xs">{t.owner ?? "Your target"}</span>
      </div>
      <Progress current={t.current} total={t.total} tone={t.tone} />
      <span className="w-28 text-right">
        <Chip tone={t.tone}>{t.verdict}</Chip>
      </span>
      <Button variant="ghost" icon={<IconDots />} aria-label="More" />
    </div>
  ),
  run: (r) => (
    <div className={`${ROW} items-center`}>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="font-medium">{r.title}</span>
        <span className="truncate text-[var(--ink-2)] text-xs">{r.meta}</span>
      </div>
      <span className="w-32 text-[var(--ink-2)] text-xs tabular-nums">{r.when}</span>
      <span className="w-20 text-right">
        <Chip tone={r.tone}>{r.outcome}</Chip>
      </span>
      <Info text={`Run reference ${r.reference}`} />
    </div>
  ),
};

/** D · One line: name, its verdict chip beside it, the rest muted on the same line; detail and reference behind ⓘ. */
const ONE_LINE: Layout = {
  phase: (p) => (
    <div className={`${ROW} items-center`}>
      <span className="font-semibold">{p.label}</span>
      <Chip tone={p.tone}>{p.verdict}</Chip>
      {p.guidance ? <Info text={`${p.guidance.headline} ${p.guidance.remediation}`} /> : null}
      <span className="min-w-0 flex-1 truncate text-[var(--ink-2)] text-xs">{p.meta}</span>
      <span className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
        <Switch checked={p.on} />
        {p.cadence}
      </span>
      <Button variant="outline" icon={<IconPlayerPlay />} aria-label="Run now" />
    </div>
  ),
  target: (t) => (
    <div className={`${ROW} items-center`}>
      <span className="font-semibold">{t.id}</span>
      <Chip tone={t.tone}>{t.verdict}</Chip>
      {t.guidance ? <Info text={`${t.guidance.headline} ${t.guidance.remediation}`} /> : null}
      <span className="min-w-0 flex-1 truncate text-[var(--ink-2)] text-xs">
        {t.current} of {t.total} routes{t.owner ? ` · ${t.owner}` : ""}
      </span>
      {t.connect ? (
        <Button variant="default">Connect</Button>
      ) : (
        <Button variant="outline" icon={<IconRefresh />} aria-label="Reconcile" />
      )}
    </div>
  ),
  run: (r) => (
    <div className={`${ROW} items-center`}>
      <span className="w-32 shrink-0 text-[var(--ink-2)] text-xs tabular-nums">{r.when}</span>
      <span className="font-medium">{r.title}</span>
      <span className="min-w-0 flex-1 truncate text-[var(--ink-2)] text-xs">{r.meta}</span>
      <Chip tone={r.tone}>{r.outcome}</Chip>
      <Info text={`Run reference ${r.reference}`} />
    </div>
  ),
};

function Cards({ layout }: { layout: Layout }) {
  const icon = (Glyph: typeof IconCheck) => mark(Glyph);
  return (
    <div className="flex min-h-dvh flex-col gap-5 bg-[var(--base)] p-8 text-[var(--ink)]">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <Panel icon={icon(IconPlayerPlay)} title="Now">
          <div className={INSET}>
            {PHASES.map((p) => (
              <Fragment key={p.label}>{layout.phase(p)}</Fragment>
            ))}
          </div>
        </Panel>
        <Panel icon={icon(IconTarget)} title="What the targets hold">
          <div className={INSET}>
            {TARGETS.map((t) => (
              <Fragment key={t.id}>{layout.target(t)}</Fragment>
            ))}
          </div>
        </Panel>
        <Panel icon={icon(IconHistory)} title="What has happened">
          <div className={INSET}>
            {RUNS.map((r) => (
              <Fragment key={r.reference}>{layout.run(r)}</Fragment>
            ))}
          </div>
        </Panel>
        <Panel icon={icon(IconListCheck)} title="Task history">
          <div className={INSET}>
            {TASKS.map((r) => (
              <Fragment key={r.reference}>{layout.run(r)}</Fragment>
            ))}
          </div>
        </Panel>
      </div>
    </div>
  );
}

const meta = {
  title: "Spikes/Row Content",
  component: Cards,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Cards>;

export default meta;
type Story = StoryObj<typeof meta>;

export const A_Today: Story = { args: { layout: TODAY } };
export const B_StatusMark: Story = { args: { layout: MARKED } };
export const C_FigureColumns: Story = { args: { layout: COLUMNS } };
export const D_OneLine: Story = { args: { layout: ONE_LINE } };
