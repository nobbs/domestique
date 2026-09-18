import { Tabs } from "@base-ui/react/tabs";
import type { ReactNode } from "react";
import { Navigate, useLocation, useNavigate, useParams } from "react-router";
import { cn } from "@/lib/utils";
import { SegmentedTrack, SegmentLabel, segmentClass } from "./Segmented";

export interface PageTab {
  /** The path segment the tab lives at, under the page's own path. */
  key: string;
  label: string;
  icon?: ReactNode;
  content: ReactNode;
}

/**
 * A page's sections as segmented tabs, each at `base/key`. A missing or unknown
 * segment lands on the first tab, keeping the query string it arrived with.
 */
export function PageTabs({
  label,
  base,
  tabs,
}: {
  /** Names the tab list for assistive technology. */
  label: string;
  base: string;
  tabs: readonly PageTab[];
}) {
  const { section } = useParams();
  const { search } = useLocation();
  const navigate = useNavigate();
  const current = tabs.find((tab) => tab.key === section);
  const first = tabs[0];

  if (!current) {
    return first ? <Navigate to={`${base}/${first.key}${search}`} replace /> : null;
  }

  return (
    <Tabs.Root
      value={current.key}
      onValueChange={(next) => navigate(`${base}/${String(next)}`)}
      className="flex flex-col gap-5"
    >
      <SegmentedTrack active={current.key} className="max-w-full overflow-x-auto">
        <Tabs.List aria-label={label} className="contents">
          {tabs.map((tab) => (
            <Tabs.Tab
              key={tab.key}
              value={tab.key}
              data-segment={tab.key}
              className={cn(
                segmentClass(),
                "data-[active]:font-semibold data-[active]:text-[var(--ink)]",
              )}
            >
              {tab.icon}
              <SegmentLabel>{tab.label}</SegmentLabel>
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </SegmentedTrack>
      <Tabs.Panel value={current.key} className="flex flex-col gap-5 outline-none">
        {current.content}
      </Tabs.Panel>
    </Tabs.Root>
  );
}
