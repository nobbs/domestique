import { Navigate, Route, Routes, useParams } from "react-router";
import { RoutesPage } from "./features/routes/RoutesPage";
import { SyncPage } from "./features/sync/SyncPage";

/**
 * The address a route used to have, answered by the one it has now.
 *
 * A route is no longer a page: it is the entry page with a route open, carried
 * in the query so it stays linkable. Anything that already held the old path —
 * a bookmark, a link in a note — lands on the same route rather than on a
 * missing page, and the identity travels across unchanged.
 */
function OpenedRoute() {
  const { provider, routeId, stage } = useParams();
  const key = provider && routeId && stage ? `${provider}/${routeId}/${stage}` : null;

  return <Navigate to={key ? `/?route=${encodeURIComponent(key)}` : "/"} replace />;
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
  const key = routeId && stage ? `veloplanner/${routeId}/${stage}` : null;

  return <Navigate to={key ? `/?route=${encodeURIComponent(key)}` : "/"} replace />;
}

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<RoutesPage />} />
      <Route path="routes/:provider/:routeId/:stage" element={<OpenedRoute />} />
      <Route path="routes/:routeId/:stage" element={<OpenedLegacyRoute />} />
      <Route path="sync" element={<SyncPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
