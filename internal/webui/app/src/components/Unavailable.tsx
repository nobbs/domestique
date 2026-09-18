/**
 * A page that is not this reader's to see, saying so.
 *
 * Only ever shown to a reader the page could belong to — an admin previewing
 * the rider view, or one whose service has the planner switched off. Anyone
 * else is redirected instead, since a page explaining what it will not show
 * is itself the news that the thing exists.
 */

import { IconLock } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { PageShell } from "./Layout";
import { Panel } from "./PanelHeading";

export function Unavailable({
  title,
  detail,
  action,
}: {
  title: string;
  detail: ReactNode;
  action?: ReactNode;
}) {
  return (
    <PageShell>
      <div className="mx-auto w-full max-w-xl pt-10">
        <Panel icon={<IconLock size={18} stroke={1.8} />} title={title}>
          <p className="text-[var(--ink-2)] text-sm">{detail}</p>
          {action ? <div className="flex">{action}</div> : null}
        </Panel>
      </div>
    </PageShell>
  );
}
