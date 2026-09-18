import { IconRefresh } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Panel } from "../../components/PanelHeading";

/** One card: a marked heading and whatever answers it. */
export function SyncCard({
  id,
  heading,
  icon = <IconRefresh size={18} stroke={1.8} />,
  children,
}: {
  id: string;
  heading: string;
  /** The glyph in the heading's mark; sync's own when absent. */
  icon?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Panel id={`${id}-heading`} icon={icon} title={heading}>
      <div className="grid gap-4">{children}</div>
    </Panel>
  );
}
