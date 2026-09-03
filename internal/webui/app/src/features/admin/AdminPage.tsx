import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ButtonLink } from "../../components/Button";
import { PageShell } from "../../components/Layout";
import { ServiceSettings } from "./ServiceSettings";

/** The service-wide cards a rider's own `/settings` no longer holds. */
export function AdminPage() {
  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">Admin</h1>
        <ServiceSettings />
        <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
          <CardHeader>
            <CardTitle role="heading" aria-level={2}>
              Tasks
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-[var(--ink-2)]">
              What the background layer runs, its schedule, and what it has been doing.
            </p>
            <ButtonLink to="/admin/tasks">Open tasks</ButtonLink>
          </CardContent>
        </Card>
      </div>
    </PageShell>
  );
}
