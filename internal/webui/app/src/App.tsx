import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useLayoutEffect } from "react";
import { Navigate, Route, Routes, useParams } from "react-router";
import { webUIConfigQuery } from "./api/queries";
import { AdminPage } from "./features/admin/AdminPage";
import { TasksPage } from "./features/admin/tasks/TasksPage";
import { SignInPage } from "./features/auth/SignInPage";
import { CataloguePage } from "./features/catalogue/CataloguePage";
import { AtlasPage } from "./features/routes/AtlasPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { SyncPage } from "./features/sync/SyncPage";
import { useEffectiveAdmin } from "./lib/identity";
import { useThemeChoice } from "./lib/theme";

/**
 * Guards an admin-only route. Nothing is rendered while identity is still
 * loading — deciding early would bounce an admin to `/settings` on first
 * paint, before their own config has even arrived.
 */
function AdminOnly({ children }: { children: ReactNode }) {
  const { isPending } = useQuery(webUIConfigQuery());
  const effectiveAdmin = useEffectiveAdmin();

  if (isPending) {
    return null;
  }

  return effectiveAdmin ? children : <Navigate to="/settings" replace />;
}

/**
 * The address a route used to have, answered by the one it has now.
 *
 * A route is no longer a page: it is the entry page with a route open, carried
 * in the query so it stays linkable. Anything that already held the old path —
 * a bookmark, a link in a note — lands on the same route rather than on a
 * missing page, and the identity travels across unchanged.
 *
 * Nothing here checks that the segments are present: this renders only because
 * the pattern below matched, and a pattern matches only with every dynamic
 * segment filled. What the address says is a route is checked where it can
 * actually be wrong — against the library, once the query is read back.
 */
function OpenedRoute() {
  const { provider, sourceRouteId, stageOrder } = useParams();
  const key = `${provider}/${sourceRouteId}/${stageOrder}`;

  return <Navigate to={`/?route=${encodeURIComponent(key)}`} replace />;
}

/**
 * The address a route had before a second provider gave every route a
 * provider of its own, answered the same way.
 *
 * Only VeloPlanner ever handed out a two-segment link, so the provider a link
 * like this named is the one it always meant — the same assumption the Go
 * handler makes for the same paths in production.
 */
function OpenedLegacyRoute() {
  const { sourceRouteId, stageOrder } = useParams();
  const key = `veloplanner/${sourceRouteId}/${stageOrder}`;

  return <Navigate to={`/?route=${encodeURIComponent(key)}`} replace />;
}

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 *
 * The theme choice lives here rather than in `AtlasPage`, even though only
 * Settings offers the control for it: the palette it switches is `index.css`'s
 * own, read by every page, and `data-theme` is a document-level attribute —
 * there is exactly one of it, whichever page happens to be mounted.
 */
export function App() {
  const [themeChoice, setThemeChoice] = useThemeChoice();

  // Layout rather than passive: this app renders nothing until React mounts,
  // so the commit this runs after is the first paint there is — an ordinary
  // effect would let the browser paint the system default first and flash to
  // a remembered override a frame later.
  useLayoutEffect(() => {
    if (themeChoice === "system") {
      document.documentElement.removeAttribute("data-theme");
    } else {
      document.documentElement.dataset.theme = themeChoice;
    }
  }, [themeChoice]);

  return (
    <Routes>
      <Route path="/" element={<AtlasPage themeChoice={themeChoice} />} />
      <Route path="routes/:provider/:sourceRouteId/:stageOrder" element={<OpenedRoute />} />
      <Route path="routes/:sourceRouteId/:stageOrder" element={<OpenedLegacyRoute />} />
      <Route path="catalogue" element={<CataloguePage />} />
      {/* The one page reached without a session. The service serves this same
          document there, so the sign-in form is the application's own. */}
      <Route path="auth/login" element={<SignInPage />} />
      <Route path="sync" element={<SyncPage />} />
      <Route
        path="settings"
        element={<SettingsPage themeChoice={themeChoice} onThemeChoiceChange={setThemeChoice} />}
      />
      <Route
        path="admin"
        element={
          <AdminOnly>
            <AdminPage />
          </AdminOnly>
        }
      />
      <Route
        path="admin/tasks"
        element={
          <AdminOnly>
            <TasksPage />
          </AdminOnly>
        }
      />
      <Route path="settings/tasks" element={<Navigate to="/admin/tasks" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
