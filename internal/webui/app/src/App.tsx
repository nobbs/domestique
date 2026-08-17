import { Navigate, Route, Routes } from "react-router";
import { StageDetail } from "./features/routes/StageDetail";
import { StagesPage } from "./features/routes/StagesPage";

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<StagesPage />}>
        <Route path="routes/:routeId/:stage" element={<StageDetail />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
