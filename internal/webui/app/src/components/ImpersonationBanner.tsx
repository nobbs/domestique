import { IconEye } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { webUIConfigQuery } from "../api/queries";
import { stopImpersonating } from "../lib/identity";

/** Says, across every page, that this admin is browsing as someone else. */
export function ImpersonationBanner() {
  const { data } = useQuery(webUIConfigQuery());
  const identity = data?.identity;
  if (!identity?.impersonating) {
    return null;
  }

  return (
    <div
      role="status"
      className="flex shrink-0 items-center gap-2 border-[var(--rule)] border-b bg-[color-mix(in_srgb,var(--hold)_15%,var(--panel))] px-3 py-1.5 text-sm sm:px-4"
    >
      <IconEye aria-hidden="true" className="shrink-0 text-[var(--hold)]" size={16} stroke={1.6} />
      <span className="min-w-0 flex-1 truncate">
        Viewing as <strong className="font-semibold">{identity.display}</strong>
      </span>
      <button
        type="button"
        className="shrink-0 font-semibold text-[var(--hold)] underline-offset-4 hover:underline"
        onClick={() => void stopImpersonating()}
      >
        Stop
      </button>
    </div>
  );
}
