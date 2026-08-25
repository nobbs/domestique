import { useState } from "react";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import { useNarrowViewport } from "../lib/mediaQuery";
import { Wordmark } from "./brand/Wordmark";

export interface LayoutProps {
  /** The full-bleed library map. */
  map?: React.ReactNode;
  /** Search, filters, results, and route detail. */
  children?: React.ReactNode;
}

/** The map stays mounted while the same workspace becomes a rail or a Drawer. */
export function Layout({ map, children }: LayoutProps) {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const narrow = useNarrowViewport();

  return (
    <main className="relative isolate h-dvh overflow-hidden bg-[var(--ground)] text-[var(--ink)]">
      <div className="absolute inset-0">{map}</div>
      <header className="absolute top-3 left-3 z-30">
        <Wordmark />
      </header>
      {narrow ? (
        <div className="absolute right-3 bottom-[calc(0.75rem+env(safe-area-inset-bottom))] z-30">
          <Drawer open={drawerOpen} onOpenChange={setDrawerOpen} showSwipeHandle>
            <DrawerTrigger className="rounded-lg bg-[var(--panel)] px-3 py-2 text-sm font-semibold shadow-[var(--shadow)] ring-1 ring-black/5">
              Browse routes
            </DrawerTrigger>
            <DrawerContent className="bg-[var(--panel)] text-[var(--ink)]">
              <DrawerHeader className="sr-only">
                <DrawerTitle>Route library</DrawerTitle>
              </DrawerHeader>
              <div className="min-h-0 overflow-y-auto p-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))]">
                {children}
              </div>
            </DrawerContent>
          </Drawer>
        </div>
      ) : (
        <div className="shell__overlay pointer-events-none absolute inset-0 z-20">
          <aside
            className="pointer-events-auto absolute top-16 left-3 max-h-[calc(100dvh-4.75rem)] w-fit max-w-[calc(100dvw-1.5rem)] overflow-y-auto rounded-xl bg-[var(--panel)] p-3 shadow-[var(--shadow)] ring-1 ring-black/5 transition-[background-color,box-shadow,padding] duration-200 has-[>[data-compact-workspace]]:bg-transparent has-[>[data-compact-workspace]]:p-0 has-[>[data-compact-workspace]]:shadow-none has-[>[data-compact-workspace]]:ring-0"
            aria-label="Route library controls"
          >
            {children}
          </aside>
        </div>
      )}
    </main>
  );
}

/** A page that is read rather than looked at. */
export function PageShell({ children }: { children: React.ReactNode }) {
  return (
    <main className="min-h-dvh bg-[var(--base)] px-4 py-6 text-[var(--ink)] sm:px-6 sm:py-8">
      {children}
    </main>
  );
}
