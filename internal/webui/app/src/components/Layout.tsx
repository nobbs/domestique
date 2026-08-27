import { useState } from "react";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import { useNarrowViewport } from "../lib/mediaQuery";
import { MenuBar } from "./MenuBar";

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
    /*
     * A column, not a stack: the bar takes its own height off the top and the
     * map has what is left. Nothing measures the bar and holds the camera out
     * from under it, because the map's frame no longer reaches it.
     */
    <div className="flex h-dvh flex-col bg-[var(--ground)] text-[var(--ink)]">
      <MenuBar />
      <main className="relative isolate min-h-0 flex-1 overflow-hidden">
        <div className="absolute inset-0">{map}</div>
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
              className="pointer-events-auto absolute top-3 left-3 max-h-[calc(100%-1.5rem)] w-fit max-w-[calc(100dvw-1.5rem)] overflow-y-auto rounded-xl bg-[var(--panel)] p-3 shadow-[var(--shadow)] ring-1 ring-black/5 transition-[background-color,box-shadow,padding] duration-200 has-[>[data-compact-workspace]]:overflow-visible has-[>[data-compact-workspace]]:bg-transparent has-[>[data-compact-workspace]]:p-0 has-[>[data-compact-workspace]]:shadow-none has-[>[data-compact-workspace]]:ring-0"
              aria-label="Route library controls"
            >
              {children}
            </aside>
          </div>
        )}
      </main>
    </div>
  );
}

/** A page that is read rather than looked at. */
export function PageShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col bg-[var(--base)] text-[var(--ink)]">
      <MenuBar />
      <main className="flex-1 px-4 py-6 sm:px-6 sm:py-8">{children}</main>
    </div>
  );
}
