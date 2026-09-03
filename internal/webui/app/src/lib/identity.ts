/**
 * Whether an admin session is browsing as itself or previewing what a rider
 * sees.
 *
 * The toggle is a UI preview only: the server still admits an admin session
 * regardless of it, so this is not a privilege drop. Remembered the same way
 * `units.ts` remembers a unit system — one namespaced `localStorage` key,
 * guarded against a browser that refuses storage outright.
 */

import { useQuery } from "@tanstack/react-query";
import { useCallback, useSyncExternalStore } from "react";
import { webUIConfigQuery } from "../api/queries";

const STORAGE_KEY = "domestique.viewAsRider";

// Consumers share one screen (unlike units.ts's), so a flip must reach
// every one of them at once rather than wait for a remount.
const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);

  return () => listeners.delete(listener);
}

/** The admin's own preview choice, remembered across visits and shared live across every consumer. */
export function useViewAsRider(): [boolean, (value: boolean) => void] {
  const value = useSyncExternalStore(subscribe, readViewAsRider);

  const choose = useCallback((next: boolean) => {
    writeViewAsRider(next);
    for (const listener of listeners) {
      listener();
    }
  }, []);

  return [value, choose];
}

// Held in memory too, so a storage that throws still lets the toggle flip
// for the page that flipped it.
let current = false;

function readViewAsRider(): boolean {
  try {
    current = window.localStorage.getItem(STORAGE_KEY) === "true";
  } catch {}

  return current;
}

function writeViewAsRider(value: boolean): void {
  current = value;
  try {
    window.localStorage.setItem(STORAGE_KEY, String(value));
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}

/**
 * `identity.admin`, minus a "view as rider" preview the admin has switched
 * on. False while identity is still loading, so every branch that reads this
 * waits on the same answer rather than flashing an admin-only control first.
 *
 * The one place that must keep reading `identity.admin` directly is the
 * switch itself — otherwise it would vanish the moment it is flipped on.
 */
export function useEffectiveAdmin(): boolean {
  const { data } = useQuery(webUIConfigQuery());
  const [viewAsRider] = useViewAsRider();

  return (data?.identity.admin ?? false) && !viewAsRider;
}
