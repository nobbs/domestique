/**
 * The application shell: brand header and a content area. It knows nothing
 * about routes or stages, so later features reuse it as-is.
 */

import { Link } from "react-router";
import { Logo } from "./Logo";

export interface LayoutProps {
  children: React.ReactNode;
  status?: React.ReactNode;
}

export function Layout({ children, status }: LayoutProps) {
  return (
    <div className="layout">
      <header className="layout__header">
        <Link to="/" className="layout__brand">
          <Logo size={26} />
          <span className="layout__wordmark">domestique</span>
        </Link>
        {status ? <div className="layout__status">{status}</div> : null}
      </header>
      <main className="layout__main">{children}</main>
    </div>
  );
}
