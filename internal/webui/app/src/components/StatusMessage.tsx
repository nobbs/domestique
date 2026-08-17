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
    <div className="status-message" data-tone={tone} role={tone === "error" ? "alert" : "status"}>
      <p className="status-message__title">{title}</p>
      {detail ? <p className="status-message__detail">{detail}</p> : null}
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
