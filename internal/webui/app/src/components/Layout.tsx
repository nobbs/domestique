/**
 * The application shell: brand header, a sidebar slot, and a main content slot.
 * It knows nothing about routes or stages, so later features reuse it as-is.
 */

import { Link } from "react-router";
import { Logo } from "./Logo";

export interface LayoutProps {
  sidebar: React.ReactNode;
  children: React.ReactNode;
  status?: React.ReactNode;
}

export function Layout({ sidebar, children, status }: LayoutProps) {
  return (
    <div className="layout">
      <header className="layout__header">
        <Link to="/" className="layout__brand">
          <Logo size={26} />
          <span className="layout__wordmark">domestique</span>
        </Link>
        {status ? <div className="layout__status">{status}</div> : null}
      </header>
      <div className="layout__body">
        <aside className="layout__sidebar" aria-label="Route stages">
          {sidebar}
        </aside>
        <main className="layout__main">{children}</main>
      </div>
    </div>
  );
}
