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
  const { routeId, stage } = useParams();
  const key = routeId && stage ? `${routeId}/${stage}` : null;

  return <Navigate to={key ? `/?route=${encodeURIComponent(key)}` : "/"} replace />;
}

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 *
 * Two of them, and only two: the library, and the machinery behind it.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<RoutesPage />} />
      <Route path="routes/:routeId/:stage" element={<OpenedRoute />} />
      <Route path="sync" element={<SyncPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
