import type { ReactNode } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "../../components/ui/card";

/** One card: a heading and whatever answers it. */
export function SyncCard({
  id,
  heading,
  children,
}: {
  id: string;
  heading: string;
  children: ReactNode;
}) {
  return (
    <Card
      className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]"
      role="region"
      aria-labelledby={`${id}-heading`}
    >
      <CardHeader className="pb-3">
        <CardTitle id={`${id}-heading`} role="heading" aria-level={2}>
          {heading}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">{children}</CardContent>
    </Card>
  );
}
