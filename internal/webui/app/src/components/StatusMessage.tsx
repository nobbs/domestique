/**
 * The shared empty, loading, and failure presentation. Features compose this
 * rather than each inventing their own placeholder.
 */

export type StatusTone = "neutral" | "error";

export interface StatusMessageProps {
  title: string;
  detail?: string;
  tone?: StatusTone;
  children?: React.ReactNode;
}

export function StatusMessage({ title, detail, tone = "neutral", children }: StatusMessageProps) {
  return (
    <div
      className={`rounded-lg border p-3 text-sm shadow-[var(--shadow)] ${tone === "error" ? "border-[var(--alert)]/30 bg-[var(--panel)] text-[var(--alert)]" : "border-[var(--rule)] bg-[var(--panel)] text-[var(--ink)]"}`}
      role={tone === "error" ? "alert" : "status"}
    >
      <p className="font-semibold">{title}</p>
      {detail ? <p className="mt-1 text-[var(--ink-2)]">{detail}</p> : null}
      {children}
    </div>
  );
}

export function LoadingMessage({ what }: { what: string }) {
  return <StatusMessage title={`Loading ${what}…`} />;
}

export function ErrorMessage({ what, error }: { what: string; error: unknown }) {
  const detail = error instanceof Error ? error.message : undefined;

  return (
    <StatusMessage tone="error" title={`Could not load ${what}.`} {...(detail ? { detail } : {})} />
  );
}
