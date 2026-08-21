import { Navigate, Route, Routes } from "react-router";
import { RouteDetail } from "./features/routes/RouteDetail";
import { RoutesPage } from "./features/routes/RoutesPage";
import { SyncPage } from "./features/sync/SyncPage";

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 *
 * A route's identity is the pair the service serves it under, so it keeps both
 * halves of that pair in the address even though the page never shows the
 * second: a library with more than one stage under a route would otherwise have
 * pages that cannot be linked to.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<RoutesPage />} />
      <Route path="routes/:routeId/:stage" element={<RouteDetail />} />
      <Route path="sync" element={<SyncPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
