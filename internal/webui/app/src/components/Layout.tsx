/**
 * The two shells every page is built on.
 *
 * There is no header. The entry page is a map of the whole library, and a bar
 * across the top of it would be a strip of chrome over the one thing worth
 * looking at, so the shell is a full-bleed pane with a slot for the panels that
 * float above it. What goes in that slot is the page's business; the shell only
 * says where the map is and where the panels may sit.
 *
 * The sync page is not a map, so it gets the second shell: a plain scrolling
 * surface. Both know nothing about routes or runs.
 */

export interface LayoutProps {
  /** The full-bleed pane. Everything else floats over it. */
  map?: React.ReactNode;
  /** The floating panels, top-left on the desktop and docked low on a phone. */
  children?: React.ReactNode;
  /**
   * Whether a panel has grown into something tall — a results column, say.
   *
   * On a phone the panels sit at the bottom of the pane, within thumb's reach.
   * A column growing downward from there would grow straight off the screen, so
   * an expanded overlay rises to the top instead.
   */
  expanded?: boolean;
}

export function Layout({ map, children, expanded = false }: LayoutProps) {
  return (
    /*
     * The shell is the page's one main landmark. The map is content rather than
     * chrome here — it is the library — so it sits inside it with the panels
     * that float over it, and nothing on the page is left outside a landmark.
     */
    <main className="shell">
      <div className="shell__map">{map}</div>
      <div className="shell__overlay" data-expanded={expanded}>
        {children}
      </div>
    </main>
  );
}

/** A page that is read rather than looked at: it scrolls, and has no map. */
export function PageShell({ children }: { children: React.ReactNode }) {
  return <main className="shell shell--page">{children}</main>;
}
