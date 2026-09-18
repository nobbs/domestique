/**
 * Four treatments for the settings forms, shown on the rider profile. Storybook
 * only: values are written in and nothing saves.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconBell,
  IconBooks,
  IconChevronDown,
  IconChevronUp,
  IconDeviceGamepad2,
  IconInfoCircle,
  IconMinus,
  IconPlugConnected,
  IconPlus,
  IconRefresh,
  IconSparkles,
  IconUser,
} from "@tabler/icons-react";
import { Fragment, type ReactNode, useState } from "react";
import { Button } from "@/components/Button";
import { FormGroup, FormRow } from "@/components/InsetForm";
import { Panel } from "@/components/PanelHeading";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

interface Parameter {
  label: string;
  unit: string;
  hint: string;
  value: string;
  suggested?: string;
}

const GROUPS: { title: string; parameters: Parameter[] }[] = [
  {
    title: "Heart rate",
    parameters: [
      {
        label: "Maximum",
        unit: "bpm",
        hint: "The highest rate you reach, which the top of every zone is a share of.",
        value: "185",
        suggested: "171",
      },
      {
        label: "Resting",
        unit: "bpm",
        hint: "Your rate at rest, which with the maximum gives the reserve that zones are cut from.",
        value: "45",
      },
      {
        label: "Threshold",
        unit: "bpm",
        hint: "The lactate threshold rate, where a zone scheme cuts hard from moderate.",
        value: "148",
        suggested: "149",
      },
    ],
  },
  {
    title: "Power",
    parameters: [
      {
        label: "Functional threshold power",
        unit: "W",
        hint: "The power you hold for an hour, which every ride's load is measured against.",
        value: "249",
        suggested: "256",
      },
    ],
  },
  {
    title: "Rider and bicycle",
    parameters: [
      { label: "Rider mass", unit: "kg", hint: "You, dressed to ride.", value: "74.5" },
      {
        label: "Bike mass",
        unit: "kg",
        hint: "The bicycle and everything carried on it.",
        value: "9.2",
      },
      {
        label: "Drag area",
        unit: "m²",
        hint: "CdA: 0.36 on a road bike's hoods, 0.40 on a gravel bike's hoods, 0.45 sitting up.",
        value: "0.40",
      },
      {
        label: "Rolling resistance",
        unit: "Crr",
        hint: "0.005 for a road slick, 0.008 for a wide gravel tyre.",
        value: "0.005",
      },
    ],
  },
];

const WASH = "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";
const FOCUS =
  "focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-[var(--accent)]";

function Card({ children }: { children: ReactNode }) {
  return (
    <div className="max-w-4xl p-6">
      <Panel icon={<IconUser size={18} stroke={1.8} />} title="Rider profile">
        {children}
      </Panel>
    </div>
  );
}

function Save() {
  return (
    <div className="flex items-center justify-end gap-3 pt-2">
      <span className="text-[var(--ink-2)] text-xs">
        Empty fields are numbers the service lacks
      </span>
      <Button variant="default">Save</Button>
    </div>
  );
}

/** A compact number with its unit set inside the field, right-aligned like a figure. */
function UnitInput({ p, className = "w-36" }: { p: Parameter; className?: string }) {
  const [value, setValue] = useState(p.value);
  return (
    <label
      className={`flex h-9 items-center rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] pr-3 ${FOCUS} ${className}`}
    >
      <span className="sr-only">{p.label}</span>
      <input
        inputMode="decimal"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="h-full min-w-0 flex-1 bg-transparent pl-3 text-right font-medium tabular-nums outline-none"
      />
      <span className="ml-1.5 min-w-8 text-[var(--ink-2)] text-xs">{p.unit}</span>
    </label>
  );
}

function Suggest({ p }: { p: Parameter }) {
  return p.suggested ? (
    <button
      type="button"
      className="inline-flex items-center gap-1 rounded-full bg-[color-mix(in_oklab,var(--primary)_10%,transparent)] px-2 py-0.5 text-xs hover:bg-[color-mix(in_oklab,var(--primary)_18%,transparent)]"
    >
      <IconSparkles size={12} aria-hidden="true" />
      Use {p.suggested}
    </button>
  ) : null;
}

function Hint({ text }: { text: string }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={<button type="button" aria-label="About this field" />}
        className="text-[var(--ink-2)] hover:text-[var(--ink)]"
      >
        <IconInfoCircle size={14} stroke={1.8} />
      </TooltipTrigger>
      <TooltipContent className="max-w-64">{text}</TooltipContent>
    </Tooltip>
  );
}

/** A · Today: every field full width, label over input over description. */
function Today() {
  return (
    <div className="flex flex-col gap-6">
      {GROUPS.flatMap((g) => g.parameters).map((p) => (
        <div key={p.label} className="flex flex-col gap-2">
          <span className="font-semibold text-sm">
            {p.label} ({p.unit})
          </span>
          <input
            defaultValue={p.value}
            className="h-10 rounded-full border border-[var(--rule)] px-4"
          />
          <span className="text-[var(--ink-2)] text-sm">
            {p.hint}
            {p.suggested ? ` Your rides of the last 90 days suggest ${p.suggested} ${p.unit}.` : ""}
          </span>
        </div>
      ))}
    </div>
  );
}

/** B · Ledger: grouped rows, words on the left, a compact field on the right. */
function Ledger() {
  return (
    <div className="flex flex-col gap-6">
      {GROUPS.map((g) => (
        <section key={g.title} className="flex flex-col">
          <h4 className="border-[var(--rule)] border-b pb-2 font-semibold text-[var(--ink-2)] text-xs uppercase tracking-wide">
            {g.title}
          </h4>
          {g.parameters.map((p) => (
            <div
              key={p.label}
              className="grid items-center gap-x-6 gap-y-2 border-[var(--rule)] border-b py-3 last:border-b-0 sm:grid-cols-[1fr_auto]"
            >
              <div className="flex flex-col">
                <span className="font-medium text-sm">{p.label}</span>
                <span className="text-[var(--ink-2)] text-xs">{p.hint}</span>
              </div>
              <div className="flex items-center gap-2 sm:justify-end">
                <Suggest p={p} />
                <UnitInput p={p} />
              </div>
            </div>
          ))}
        </section>
      ))}
      <Save />
    </div>
  );
}

/** C · Inset groups: the account lists' wash, hints behind an info mark. */
function Inset() {
  return (
    <div className="flex flex-col gap-5">
      {GROUPS.map((g) => (
        <section key={g.title} className="flex flex-col gap-2">
          <h4 className="px-1 font-semibold text-sm">{g.title}</h4>
          <div className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
            {g.parameters.map((p) => (
              <div
                key={p.label}
                className="flex flex-wrap items-center gap-3 border-[var(--panel)] border-b-2 px-3.5 py-2.5 last:border-b-0"
              >
                <span className="flex flex-1 items-center gap-1.5 text-sm">
                  {p.label}
                  <Hint text={p.hint} />
                </span>
                {p.suggested ? (
                  <span className="text-[var(--ink-2)] text-xs">
                    rides suggest <Suggest p={p} />
                  </span>
                ) : null}
                <UnitInput p={p} className="w-32" />
              </div>
            ))}
          </div>
        </section>
      ))}
      <Save />
    </div>
  );
}

/** D · Figure tiles: each value a large editable figure, like the dashboard's readouts. */
function Tiles() {
  return (
    <div className="flex flex-col gap-5">
      {GROUPS.map((g) => (
        <section key={g.title} className="flex flex-col gap-2">
          <h4 className="px-1 font-semibold text-[var(--ink-2)] text-xs uppercase tracking-wide">
            {g.title}
          </h4>
          <div className="grid grid-cols-2 gap-2.5 md:grid-cols-4">
            {g.parameters.map((p) => (
              <Fragment key={p.label}>
                <label
                  className={`flex flex-col gap-1 rounded-xl p-3.5 ${WASH} ${FOCUS} focus-within:bg-[var(--panel)] focus-within:shadow-[inset_0_0_0_1px_var(--rule)]`}
                >
                  <span className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
                    {p.label}
                    <Hint text={p.hint} />
                  </span>
                  <span className="flex items-baseline gap-1">
                    <input
                      defaultValue={p.value}
                      inputMode="decimal"
                      className="w-full min-w-0 bg-transparent font-semibold text-2xl tabular-nums outline-none"
                    />
                    <span className="text-[var(--ink-2)] text-sm">{p.unit}</span>
                  </span>
                  <span className="min-h-5">
                    <Suggest p={p} />
                  </span>
                </label>
              </Fragment>
            ))}
          </div>
        </section>
      ))}
      <Save />
    </div>
  );
}

const VARIANTS = { today: Today, ledger: Ledger, inset: Inset, tiles: Tiles } as const;

function Spike({ variant }: { variant: keyof typeof VARIANTS }) {
  const Body = VARIANTS[variant];
  return (
    <Card>
      <Body />
    </Card>
  );
}

type Row =
  | { kind: "text"; label: string; value: string; hint?: string; mono?: boolean }
  | { kind: "secret"; label: string; set: boolean; hint?: string }
  | { kind: "number"; label: string; value: string; unit: string; hint?: string }
  | { kind: "switch"; label: string; on: boolean; hint?: string; danger?: boolean };

interface InsetForm {
  title: string;
  subtitle?: string;
  icon: ReactNode;
  note?: string;
  groups: { title?: string; rows: Row[] }[];
}

const FORMS: Record<string, InsetForm> = {
  zwift: {
    title: "Zwift account",
    icon: <IconDeviceGamepad2 size={18} stroke={1.8} />,
    note: "Zwift's API is unofficial and not supported by Zwift; your account's terms still apply.",
    groups: [
      {
        rows: [
          { kind: "secret", label: "Email", set: true },
          { kind: "secret", label: "Password", set: true },
        ],
      },
    ],
  },
  wahoo: {
    title: "Wahoo application",
    subtitle: "the registered application this service writes routes with",
    icon: <IconPlugConnected size={18} stroke={1.8} />,
    groups: [
      {
        title: "Endpoints",
        rows: [
          { kind: "text", label: "API address", value: "https://api.wahooligan.com", mono: true },
          {
            kind: "text",
            label: "Authorization address",
            value: "https://api.wahooligan.com",
            mono: true,
          },
        ],
      },
      {
        title: "Credentials",
        rows: [
          { kind: "text", label: "Client ID", value: "k2b8…f91e", mono: true },
          { kind: "secret", label: "Client secret", set: true },
          {
            kind: "secret",
            label: "Webhook token",
            set: false,
            hint: "The token Wahoo carries in every webhook body.",
          },
        ],
      },
    ],
  },
  library: {
    title: "VeloPlanner",
    subtitle: "a route library this service reads",
    icon: <IconBooks size={18} stroke={1.8} />,
    groups: [
      {
        rows: [
          { kind: "switch", label: "Read this library", on: true },
          { kind: "text", label: "Address", value: "https://veloplanner.com", mono: true },
          { kind: "secret", label: "Email", set: true },
          { kind: "secret", label: "Password", set: false },
        ],
      },
    ],
  },
  sync: {
    title: "Synchronisation",
    icon: <IconRefresh size={18} stroke={1.8} />,
    groups: [
      {
        title: "Timing",
        rows: [
          {
            kind: "number",
            label: "Call the library stale after",
            value: "36",
            unit: "h",
            hint: "How long the last successful read may stand before the status page reports it stale.",
          },
          {
            kind: "number",
            label: "Wait before the first run",
            value: "1",
            unit: "min",
            hint: "Takes effect on the next restart rather than the next run.",
          },
        ],
      },
      {
        title: "Safety",
        rows: [
          { kind: "number", label: "Most routes deleted per run", value: "10", unit: "routes" },
          {
            kind: "switch",
            label: "Let an empty library delete a target's routes",
            on: false,
            danger: true,
            hint: "Stays on until you turn it off again; it does not reset after one run.",
          },
        ],
      },
    ],
  },
  alerts: {
    title: "Alerts",
    subtitle: "what is worth a notification",
    icon: <IconBell size={18} stroke={1.8} />,
    groups: [
      {
        title: "sync:source",
        rows: [
          { kind: "switch", label: "Failed", on: true },
          { kind: "switch", label: "Held by a safety gate", on: true },
        ],
      },
      {
        title: "activity:poll",
        rows: [
          { kind: "switch", label: "Failed", on: true },
          { kind: "switch", label: "Quota spent", on: false },
        ],
      },
    ],
  },
};

const FIELD = `flex h-9 items-center rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] px-3 ${FOCUS}`;

function RowControl({ row }: { row: Row }) {
  const [on, setOn] = useState(row.kind === "switch" && row.on);
  switch (row.kind) {
    case "text":
      return (
        <label className={`${FIELD} w-full sm:w-80`}>
          <span className="sr-only">{row.label}</span>
          <input
            defaultValue={row.value}
            className={`min-w-0 flex-1 bg-transparent outline-none ${row.mono ? "font-mono text-xs" : "text-sm"}`}
          />
        </label>
      );
    case "secret":
      return (
        <label className={`${FIELD} w-full gap-2 sm:w-80`}>
          <span className="sr-only">{row.label}</span>
          <input
            type="password"
            placeholder={row.set ? "Type to replace" : "Not set"}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none"
          />
          <span
            className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] ${row.set ? "bg-[color-mix(in_oklab,var(--good)_14%,transparent)] text-[var(--good)]" : "bg-[color-mix(in_oklab,var(--ink-2)_12%,transparent)] text-[var(--ink-2)]"}`}
          >
            {row.set ? "Stored" : "Empty"}
          </span>
        </label>
      );
    case "number":
      return <UnitInput p={{ label: row.label, unit: row.unit, hint: "", value: row.value }} />;
    case "switch":
      return <Switch checked={on} onCheckedChange={setOn} aria-label={row.label} />;
  }
}

function InsetFormCard({ form }: { form: InsetForm }) {
  return (
    <div className="max-w-4xl p-6">
      <Panel icon={form.icon} title={form.title} subtitle={form.subtitle}>
        <div className="flex flex-col gap-5">
          {form.note ? <p className="text-[var(--ink-2)] text-sm">{form.note}</p> : null}
          {form.groups.map((g, index) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: fixed spike content
            <section key={index} className="flex flex-col gap-2">
              {g.title ? <h4 className="px-1 font-semibold text-sm">{g.title}</h4> : null}
              <div className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
                {g.rows.map((row) => (
                  <div
                    key={row.label}
                    className="flex flex-wrap items-center gap-3 border-[var(--panel)] border-b-2 px-3.5 py-2.5 last:border-b-0"
                  >
                    <span
                      className={`flex min-w-40 flex-1 items-center gap-1.5 text-sm ${row.kind === "switch" && row.danger ? "text-[var(--alert)]" : ""}`}
                    >
                      {row.label}
                      {row.hint ? <Hint text={row.hint} /> : null}
                    </span>
                    <RowControl row={row} />
                  </div>
                ))}
              </div>
            </section>
          ))}
          <Save />
        </div>
      </Panel>
    </div>
  );
}

const meta = {
  title: "Spikes/Forms",
  component: Spike,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Spike>;

export default meta;
type Story = StoryObj<typeof meta>;

export const A_Today: Story = { args: { variant: "today" } };
export const B_Ledger: Story = { args: { variant: "ledger" } };
export const C_Inset: Story = { args: { variant: "inset" } };
export const D_Tiles: Story = { args: { variant: "tiles" } };

const inset = (form: keyof typeof FORMS): Story => ({
  args: { variant: "inset" },
  render: () => <InsetFormCard form={FORMS[form] as InsetForm} />,
});

export const C_Inset_Zwift = inset("zwift");
export const C_Inset_WahooApplication = inset("wahoo");
export const C_Inset_Library = inset("library");
export const C_Inset_Sync = inset("sync");
export const C_Inset_Alerts = inset("alerts");

/** Switch shapes, restyled from outside the vendored primitive; literal, so Tailwind sees them. */
const SWITCHES = {
  "A · Today": "",
  "B · Same size, proportional corners":
    "rounded-[5px] [&_[data-slot=switch-thumb]]:rounded-[3.5px]",
  "C · Larger, 9px corners":
    "data-[size=default]:h-6 data-[size=default]:w-10 rounded-[9px] [&_[data-slot=switch-thumb]]:rounded-[6px] [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:size-5",
  "D · Segment, inset thumb":
    "data-[size=default]:h-7 data-[size=default]:w-12 rounded-[9px] px-[3px] [&_[data-slot=switch-thumb]]:rounded-[6px] [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:size-5 [&_[data-slot=switch-thumb]]:shadow-sm [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:data-checked:translate-x-[calc(100%-4px)]",
  "D2 · Segment, 24px":
    "data-[size=default]:h-6 data-[size=default]:w-10 rounded-[8px] px-[2px] [&_[data-slot=switch-thumb]]:rounded-[6px] [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:size-[18px] [&_[data-slot=switch-thumb]]:shadow-sm [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:data-checked:translate-x-[16px]",
  "D3 · Segment, 22px":
    "data-[size=default]:h-[22px] data-[size=default]:w-9 rounded-[7px] px-[2px] [&_[data-slot=switch-thumb]]:rounded-[5px] [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:size-4 [&_[data-slot=switch-thumb]]:shadow-sm [&_[data-slot=switch-thumb]]:group-data-[size=default]/switch:data-checked:translate-x-[14px]",
} as const;

function SwitchRow({ className, label }: { className: string; label: string }) {
  const [on, setOn] = useState(true);
  const [off, setOff] = useState(false);
  return (
    <div className="flex items-center gap-3 border-[var(--panel)] border-b-2 px-3.5 py-3 last:border-b-0">
      <span className="flex-1 text-sm">{label}</span>
      <Switch
        className={className}
        checked={on}
        onCheckedChange={setOn}
        aria-label={`${label} on`}
      />
      <Switch
        className={className}
        checked={off}
        onCheckedChange={setOff}
        aria-label={`${label} off`}
      />
      <span className="inline-flex h-9 items-center rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] px-3 text-[var(--ink-2)] text-xs">
        9px field
      </span>
    </div>
  );
}

export const Switches: Story = {
  args: { variant: "inset" },
  render: () => (
    <div className="max-w-2xl p-6">
      <Panel icon={<IconBell size={18} stroke={1.8} />} title="Switch corners">
        <div className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
          {Object.entries(SWITCHES).map(([label, className]) => (
            <SwitchRow key={label} label={label} className={className} />
          ))}
        </div>
      </Panel>
    </div>
  ),
};

const NO_SPIN =
  "[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none";

/** Steps by the value's own precision, so 74.5 moves by 0.1 and 0.005 by 0.001. */
function useStepper(initial: string) {
  const [value, setValue] = useState(initial);
  const decimals = value.split(".")[1]?.length ?? 0;
  const step = (direction: 1 | -1) =>
    setValue((current) => (Number(current) + direction * 10 ** -decimals).toFixed(decimals));
  return { value, setValue, step };
}

type StepperStyle = "stacked" | "pair" | "rail";

function Stepper({ style, unit, initial }: { style: StepperStyle; unit: string; initial: string }) {
  const { value, setValue, step } = useStepper(initial);
  const input = (
    <input
      type="number"
      inputMode="decimal"
      aria-label={`Value in ${unit}`}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      className={`h-full min-w-0 flex-1 bg-transparent pl-1 text-right font-medium tabular-nums outline-none ${NO_SPIN}`}
    />
  );
  const unitText = <span className="ml-1.5 text-[var(--ink-2)] text-xs">{unit}</span>;

  if (style === "stacked") {
    return (
      <div
        className={`flex h-9 w-40 items-center rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] pr-3 pl-1 ${FOCUS}`}
      >
        {input}
        {unitText}
        <span className="order-first mr-1 flex h-7 flex-col overflow-hidden rounded-[6px] bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)]">
          <button
            type="button"
            aria-label="Increase"
            onClick={() => step(1)}
            className="grid h-1/2 w-6 place-items-center text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] hover:text-[var(--ink)]"
          >
            <IconChevronUp size={12} stroke={2.2} />
          </button>
          <button
            type="button"
            aria-label="Decrease"
            onClick={() => step(-1)}
            className="grid h-1/2 w-6 place-items-center text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] hover:text-[var(--ink)]"
          >
            <IconChevronDown size={12} stroke={2.2} />
          </button>
        </span>
      </div>
    );
  }
  if (style === "pair") {
    return (
      <div
        className={`flex h-9 w-48 items-center rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] pr-3 pl-1 ${FOCUS}`}
      >
        {input}
        {unitText}
        <span className="order-first mr-1 flex gap-0.5">
          <button
            type="button"
            aria-label="Decrease"
            onClick={() => step(-1)}
            className="grid size-7 place-items-center rounded-[6px] bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)] text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] hover:text-[var(--ink)]"
          >
            <IconMinus size={13} stroke={2.2} />
          </button>
          <button
            type="button"
            aria-label="Increase"
            onClick={() => step(1)}
            className="grid size-7 place-items-center rounded-[6px] bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)] text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] hover:text-[var(--ink)]"
          >
            <IconPlus size={13} stroke={2.2} />
          </button>
        </span>
      </div>
    );
  }
  return (
    <div
      className={`flex h-9 w-40 items-center overflow-hidden rounded-[9px] border border-[var(--rule)] bg-[var(--panel)] pr-3 ${FOCUS}`}
    >
      {input}
      {unitText}
      <span className="order-first mr-1 flex h-full flex-col border-[var(--rule)] border-r">
        <button
          type="button"
          aria-label="Increase"
          onClick={() => step(1)}
          className="grid h-1/2 w-7 place-items-center border-[var(--rule)] border-b text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)] hover:text-[var(--ink)]"
        >
          <IconChevronUp size={12} stroke={2.2} />
        </button>
        <button
          type="button"
          aria-label="Decrease"
          onClick={() => step(-1)}
          className="grid h-1/2 w-7 place-items-center text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)] hover:text-[var(--ink)]"
        >
          <IconChevronDown size={12} stroke={2.2} />
        </button>
      </span>
    </div>
  );
}

const STEPPERS: { label: string; style: StepperStyle }[] = [
  { label: "B · Stacked chevrons in a small tile", style: "stacked" },
  { label: "C · Minus and plus buttons", style: "pair" },
  { label: "D · Split rail at the edge", style: "rail" },
];

export const Steppers: Story = {
  args: { variant: "inset" },
  render: () => (
    <div className="max-w-2xl p-6">
      <Panel icon={<IconUser size={18} stroke={1.8} />} title="Number steppers">
        {STEPPERS.map(({ label, style }) => (
          <FormGroup key={style} title={label}>
            <FormRow label="Rider mass">
              <Stepper style={style} unit="kg" initial="80" />
            </FormRow>
            <FormRow label="Drag area">
              <Stepper style={style} unit="m²" initial="0.40" />
            </FormRow>
            <FormRow label="Maximum heart rate">
              <Stepper style={style} unit="bpm" initial="185" />
            </FormRow>
          </FormGroup>
        ))}
      </Panel>
    </div>
  ),
};
