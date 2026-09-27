import { useQuery } from "@tanstack/react-query";
import { webUIConfigQuery } from "../api/queries";

/**
 * `identity.admin`, false while identity is still loading so no admin-only
 * control flashes before the answer arrives.
 */
export function useAdmin(): boolean {
  const { data } = useQuery(webUIConfigQuery());

  return data?.identity.admin ?? false;
}

/** Restores the admin's own session; a refusal lands on sign-in either way. */
export async function stopImpersonating(): Promise<void> {
  await fetch("/auth/impersonate/stop", { method: "POST", credentials: "same-origin" }).catch(
    () => undefined,
  );
  window.location.assign("/");
}
