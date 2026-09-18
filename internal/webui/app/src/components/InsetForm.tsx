/**
 * A settings form as inset groups: each group one muted block, each row a label
 * with its explanation behind an info mark, and the control at the row's end.
 */

import { IconChevronDown, IconChevronUp, IconInfoCircle } from "@tabler/icons-react";
import { type ComponentProps, type ReactNode, useRef } from "react";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

/** One block of rows, under an optional heading. */
export function FormGroup({ title, children }: { title?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      {title ? <h4 className="px-1 font-semibold text-sm">{title}</h4> : null}
      <div className="flex flex-col overflow-hidden rounded-xl bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
        {children}
      </div>
    </section>
  );
}

/**
 * One setting. `htmlFor` names the control the label is for; a control that
 * names itself, such as a switch, leaves it out. `srLabel` is read after the
 * label but not shown, for a unit the control already displays.
 */
export function FormRow({
  label,
  srLabel,
  htmlFor,
  hint,
  tone,
  children,
  below,
}: {
  label: ReactNode;
  srLabel?: string | undefined;
  htmlFor?: string | undefined;
  hint?: ReactNode;
  tone?: "alert" | undefined;
  children: ReactNode;
  /** Anything that belongs to the row but not beside it, such as a preview. */
  below?: ReactNode;
}) {
  const Label = htmlFor ? "label" : "span";

  return (
    <div className="flex flex-col gap-2 border-[var(--panel)] border-b-2 px-3.5 py-2.5 last:border-b-0">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span
          className={cn(
            "flex min-w-40 flex-1 items-center gap-1.5 text-sm",
            tone === "alert" && "text-[var(--alert)]",
          )}
        >
          <Label htmlFor={htmlFor}>
            {label}
            {srLabel ? <span className="sr-only"> {srLabel}</span> : null}
          </Label>
          {hint ? <InfoDot label={label}>{hint}</InfoDot> : null}
        </span>
        {children}
      </div>
      {below}
    </div>
  );
}

/**
 * The explanation behind an info mark, and in the document for a screen reader.
 * `framed` sets the mark on the segmented control's track colour, for a card's heading.
 */
export function InfoDot({
  label,
  framed = false,
  children,
}: {
  label: ReactNode;
  framed?: boolean;
  children: ReactNode;
}) {
  const trigger = (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            aria-label={typeof label === "string" ? `About ${label}` : "About this setting"}
          />
        }
        className={cn(
          "text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2",
          framed && "grid size-9 place-items-center rounded-[12px] bg-[var(--muted)]",
        )}
      >
        <IconInfoCircle size={framed ? 16 : 14} stroke={1.8} aria-hidden="true" />
      </TooltipTrigger>
      <TooltipContent className="max-w-72">{children}</TooltipContent>
    </Tooltip>
  );

  return (
    <>
      {trigger}
      <span className="sr-only">{children}</span>
    </>
  );
}

const FIELD = "h-9 rounded-[9px] bg-[var(--panel)]";

/** A text field sized for a row's end. */
export function RowInput({ className, ...props }: ComponentProps<typeof InputGroupInput>) {
  return (
    <InputGroup className={cn(FIELD, "w-full sm:w-80", className)}>
      <InputGroupInput {...props} />
    </InputGroup>
  );
}

/**
 * Moves a number field by its own precision, 80 by 1 and 0.40 by 0.01, through
 * the native value setter so a controlled field's onChange hears it.
 */
export function stepField(input: HTMLInputElement, direction: 1 | -1) {
  const decimals = input.value.split(".")[1]?.length ?? 0;
  let next = Number(input.value || 0) + direction * 10 ** -decimals;
  if (input.min !== "") {
    next = Math.max(next, Number(input.min));
  }
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(
    input,
    next.toFixed(decimals),
  );
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

const STEP =
  "grid h-1/2 w-6 place-items-center text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)] hover:text-[var(--ink)]";

/** A number set right like a figure, its unit inside the field and its steppers at the start. */
export function UnitInput({
  unit,
  className,
  ...props
}: ComponentProps<typeof InputGroupInput> & { unit?: string }) {
  const input = useRef<HTMLInputElement>(null);
  // Out of the tab order: the field's own arrow keys already step it.
  const stepper = (direction: 1 | -1, label: string, Glyph: typeof IconChevronUp) => (
    <button
      type="button"
      tabIndex={-1}
      aria-label={label}
      className={STEP}
      onClick={() => input.current && stepField(input.current, direction)}
    >
      <Glyph size={12} stroke={2.2} aria-hidden="true" />
    </button>
  );

  return (
    <InputGroup className={cn(FIELD, "w-40", className)}>
      <InputGroupAddon align="inline-start" className="pl-1">
        <span className="flex h-7 flex-col overflow-hidden rounded-[6px] bg-[color-mix(in_oklab,var(--ink-2)_9%,transparent)]">
          {stepper(1, "Increase", IconChevronUp)}
          {stepper(-1, "Decrease", IconChevronDown)}
        </span>
      </InputGroupAddon>
      <InputGroupInput
        ref={input}
        className="text-right font-medium tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        {...props}
      />
      {unit ? (
        <InputGroupAddon align="inline-end">
          <InputGroupText className="font-normal text-xs">{unit}</InputGroupText>
        </InputGroupAddon>
      ) : null}
    </InputGroup>
  );
}

/** A write-only credential: what is stored is never shown, only whether something is. */
export function SecretInput({
  isSet,
  className,
  ...props
}: ComponentProps<typeof InputGroupInput> & { isSet: boolean }) {
  return (
    <InputGroup className={cn(FIELD, "w-full sm:w-80", className)}>
      <InputGroupInput
        type="password"
        autoComplete="off"
        placeholder={isSet ? "Stored — type to replace" : "Not set"}
        {...props}
      />
      <InputGroupAddon align="inline-end">
        <span
          className={cn(
            "rounded-full px-2 py-0.5 font-normal text-[11px]",
            isSet
              ? "bg-[color-mix(in_oklab,var(--good)_14%,transparent)] text-[var(--good)]"
              : "bg-[color-mix(in_oklab,var(--ink-2)_12%,transparent)] text-[var(--ink-2)]",
          )}
        >
          {isSet ? "Stored" : "Empty"}
        </span>
      </InputGroupAddon>
    </InputGroup>
  );
}

/** The form's own words and its actions, set at its end. */
export function FormFooter({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center justify-end gap-3">{children}</div>;
}
