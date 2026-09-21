import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useLayoutEffect } from "react";
import { Navigate, Route, Routes, useParams, useSearchParams } from "react-router";
import { webUIConfigQuery } from "./api/queries";
import { Button } from "./components/Button";
import { Unavailable } from "./components/Unavailable";
import { AccountPage } from "./features/account/AccountPage";
import { ActivitiesPage } from "./features/activity/ActivitiesPage";
import { ActivityPage } from "./features/activity/ActivityPage";
import { AdminPage } from "./features/admin/AdminPage";
import { SignInPage } from "./features/auth/SignInPage";
import { CataloguePage } from "./features/catalogue/CataloguePage";
import { FitnessPage } from "./features/fitness/FitnessPage";
import { PlanPage } from "./features/plan/PlanPage";
import { AtlasPage } from "./features/routes/AtlasPage";
import { useEffectiveAdmin, useViewAsRider } from "./lib/identity";
import { parseRouteKey, routePath } from "./lib/library";
import { useThemeChoice } from "./lib/theme";

/**
 * Guards an admin-only route. Nothing is rendered while identity is still
 * loading — deciding early would bounce an admin to `/account` on first
 * paint, before their own config has even arrived.
 */
function AdminOnly({ children }: { children: ReactNode }) {
  const { data, isPending } = useQuery(webUIConfigQuery());
  const effectiveAdmin = useEffectiveAdmin();
  const [viewAsRider, setViewAsRider] = useViewAsRider();

  if (isPending) {
    return null;
  }
  if (effectiveAdmin) {
    return children;
  }
  if (data?.identity.admin && viewAsRider) {
    return <RiderPreview onLeave={() => setViewAsRider(false)} />;
  }

  return <Navigate to="/account" replace />;
}

/**
 * What an admin sees where their own preview has taken a page away, rather
 * than the address changing under them with nothing to say why.
 */
function RiderPreview({ onLeave }: { onLeave: () => void }) {
  return (
    <Unavailable
      title="Hidden while you view as a rider"
      detail="This page belongs to an administrator, and the rider view is switched on for this browser. Leaving it brings the page straight back."
      action={
        <Button variant="default" onClick={onLeave}>
          Leave rider view
        </Button>
      }
    />
  );
}

/** The planner exists only where an admin and a routing engine do. */
function PlanningOnly({ children }: { children: ReactNode }) {
  const { data, isPending } = useQuery(webUIConfigQuery());
  const effectiveAdmin = useEffectiveAdmin();
  const [viewAsRider, setViewAsRider] = useViewAsRider();

  if (isPending) {
    return null;
  }
  if (data?.planning && effectiveAdmin) {
    return children;
  }
  if (data?.identity.admin && viewAsRider) {
    return <RiderPreview onLeave={() => setViewAsRider(false)} />;
  }
  if (data?.identity.admin) {
    return (
      <Unavailable
        title="The planner is switched off"
        detail="This service routes plans through a BRouter engine, and none is configured. Naming one as planning.brouter_url switches the planner on."
      />
    );
  }

  return <Navigate to="/" replace />;
}

/**
 * The entry address: the rider's activities, unless it is a `/?route=` link from
 * before a route had a page of its own, which lands on that route's page.
 */
function Home() {
  const [params] = useSearchParams();
  const opened = parseRouteKey(params.get("route"));

  return <Navigate to={opened ? routePath(opened) : "/activities"} replace />;
}

/**
 * The address a route had before a second provider gave every route a
 * provider of its own. Only VeloPlanner ever handed out a two-segment link, the
 * same assumption the Go handler makes for the same paths.
 */
function OpenedLegacyRoute() {
  const { sourceRouteId, stageOrder } = useParams();

  return <Navigate to={`/routes/veloplanner/${sourceRouteId}/${stageOrder}`} replace />;
}

/** Each address is a distinct draft, so an opened plan never leaks into the next one. */
function OpenedPlan() {
  const { planId } = useParams();

  return <PlanPage key={planId ?? "new"} />;
}

/**
 * The client routes. These mirror the paths the Go handler serves the entry
 * document for, so a deep link and an in-app navigation resolve identically.
 *
 * The theme choice is read here rather than in `AtlasPage`, even though the
 * control for it is in the bar: the palette it switches is `index.css`'s own,
 * read by every page, and `data-theme` is a document-level attribute — there is
 * exactly one of it, whichever page happens to be mounted.
 */
export function App() {
  const [themeChoice] = useThemeChoice();

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
      <Route path="/" element={<Home />} />
      <Route
        path="routes/:provider/:sourceRouteId/:stageOrder"
        element={<AtlasPage themeChoice={themeChoice} />}
      />
      <Route path="routes/:sourceRouteId/:stageOrder" element={<OpenedLegacyRoute />} />
      <Route path="catalogue" element={<CataloguePage themeChoice={themeChoice} />} />
      {/* The one page reached without a session. The service serves this same
          document there, so the sign-in form is the application's own. */}
      <Route path="auth/login" element={<SignInPage />} />
      <Route path="fitness" element={<FitnessPage />} />
      {/* One element for both views, so the range and ground chosen survive a switch. */}
      <Route path="activities" element={<ActivitiesPage />}>
        <Route index />
        <Route path="rides" />
      </Route>
      <Route path="activities/:activityId" element={<ActivityPage />} />
      <Route path="account" element={<AccountPage />} />
      <Route path="account/:section" element={<AccountPage />} />
      <Route
        path="plan"
        element={
          <PlanningOnly>
            <OpenedPlan />
          </PlanningOnly>
        }
      />
      <Route
        path="plan/:planId"
        element={
          <PlanningOnly>
            <OpenedPlan />
          </PlanningOnly>
        }
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
        path="admin/:section"
        element={
          <AdminOnly>
            <AdminPage />
          </AdminOnly>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
