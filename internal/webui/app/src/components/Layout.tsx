/**
 * The application shell: brand header and a content area. It knows nothing
 * about routes or stages, so later features reuse it as-is.
 */

import { Link } from "react-router";
import { Logo } from "./Logo";

/**
 * Where this service comes from.
 *
 * The one link on the page that leaves the Tailnet, and the only outbound
 * navigation the UI has at all — everything else it shows was already stored
 * locally. It is opened in a new tab with `noreferrer`, so following it neither
 * loses the route being read nor hands GitHub the private origin the reader is
 * reading it on.
 */
const REPOSITORY_URL = "https://github.com/nobbs/domestique";

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
        <div className="layout__end">
          {status ? <div className="layout__status">{status}</div> : null}
          <a
            className="layout__source"
            href={REPOSITORY_URL}
            target="_blank"
            rel="noreferrer"
            title="Source code on GitHub"
          >
            <svg viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false">
              <path
                fill="currentColor"
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.42 7.42 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"
              />
            </svg>
            {/*
             * The mark alone in the header — a second wordmark beside this
             * service's own would read as part of it. The name is carried in
             * text for anything that cannot see the mark, rather than in an
             * `aria-label`, so it is also there when the icon fails to paint.
             */}
            <span className="visually-hidden">Source code on GitHub</span>
          </a>
        </div>
      </header>
      <main className="layout__main">{children}</main>
    </div>
  );
}
