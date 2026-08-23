import { useEffect } from "react";
import { Navigate, Route, Routes, useParams } from "react-router";
import { RoutesPage } from "./features/routes/RoutesPage";
import { SyncPage } from "./features/sync/SyncPage";
import { useThemeChoice } from "./lib/theme";

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
  const { provider, routeId, stage } = useParams();
  const key = `${provider}/${routeId}/${stage}`;

  return <Navigate to={`/?route=${encodeURIComponent(key)}`} replace />;
}

/**
 * The address a route had before a second provider gave every stage a
 * provider of its own, answered the same way.
 *
 * Only VeloPlanner ever handed out a two-segment link, so the provider a link
 * like this named is the one it always meant — the same assumption the Go
 * handler makes for the same paths in production.
 */
function OpenedLegacyRoute() {
  const { routeId, stage } = useParams();
  const key = `veloplanner/${routeId}/${stage}`;

  return <Navigate to={`/?route=${encodeURIComponent(key)}`} replace />;
}

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 *
 * The theme choice lives here rather than in `RoutesPage`, even though only
 * that page offers a control for it: the palette it switches is `index.css`'s
 * own, read by every page, and `data-theme` is a document-level attribute —
 * there is exactly one of it, whichever page happens to be mounted.
 */
export function App() {
  const [themeChoice, setThemeChoice] = useThemeChoice();

  useEffect(() => {
    if (themeChoice === "system") {
      document.documentElement.removeAttribute("data-theme");
    } else {
      document.documentElement.dataset.theme = themeChoice;
    }
  }, [themeChoice]);

  return (
    <Routes>
      <Route
        path="/"
        element={<RoutesPage themeChoice={themeChoice} onThemeChoiceChange={setThemeChoice} />}
      />
      <Route path="routes/:provider/:routeId/:stage" element={<OpenedRoute />} />
      <Route path="routes/:routeId/:stage" element={<OpenedLegacyRoute />} />
      <Route path="sync" element={<SyncPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
