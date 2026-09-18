import type { ReactNode } from "react";
import { PanelHeading } from "../../components/PanelHeading";

/** One titled panel of the page, with whatever qualifies its title beside it. */
export function FitnessSection({
  icon,
  title,
  aside,
  children,
}: {
  /** The glyph in the heading's mark, drawn at 18 pixels. */
  icon: ReactNode;
  title: string;
  aside?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section
      aria-label={title}
      className="flex flex-col gap-3 min-w-0 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]"
    >
      <PanelHeading icon={icon} title={title} aside={aside} />
      {children}
    </section>
  );
}
