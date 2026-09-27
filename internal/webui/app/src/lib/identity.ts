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
