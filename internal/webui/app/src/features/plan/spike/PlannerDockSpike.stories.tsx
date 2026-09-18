/**
 * Three readings of the planner's strip beneath the map, over a synthetic
 * route measured through the real profile, surface and mix helpers, so every
 * figure shown is one the page could actually compute today.
 */

import { Tabs } from "@base-ui/react/tabs";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconArrowDownRight,
  IconArrowUpRight,
  IconClock,
  IconLayoutBottombarCollapse,
  IconMountain,
  IconRoad,
} from "@tabler/icons-react";
import { type ReactNode, useMemo, useState } from "react";
import type { Position, SurfaceRange } from "../../../api/types";
import { Button } from "../../../components/Button";
import { SegmentedTrack, SegmentLabel, segmentClass } from "../../../components/Segmented";
import { formatAscent, formatDistance, formatDuration, formatElevation } from "../../../lib/format";
import { groundSegments, steepnessEntries, surfaceEntries } from "../../../lib/mix";
import { buildProfile, gradientSharesBySign } from "../../../lib/profile";
import { summariseSurface } from "../../../lib/surface";
import { ElevationProfile } from "../../routes/ElevationProfile";
import { GroundRibbon } from "../../routes/GroundRibbon";

/** A loop with two hills, dense enough for the profile's own sampling. */
function syntheticRoute(): Position[] {
  const points: Position[] = [];
  for (let index = 0; index <= 240; index++) {
    const along = index / 240;
    const elevation =
      112 +
      70 * Math.max(0, Math.sin((along - 0.18) * Math.PI * 3.2)) ** 2 +
      40 * Math.max(0, Math.sin((along - 0.62) * Math.PI * 4)) ** 2;
    points.push([8.4 + along * 0.19, 49.0 + Math.sin(along * Math.PI * 2) * 0.02, elevation]);
  }

  return points;
}

const LINE = syntheticRoute();
// Roughly a third of the way round is gravel, with a short path near the top.
const RANGES: SurfaceRange[] = [
  { kind: "asphalt", startIndex: 0, endIndex: 78 },
  { kind: "gravel", startIndex: 78, endIndex: 132 },
  { kind: "ground", startIndex: 132, endIndex: 150 },
  { kind: "asphalt", startIndex: 150, endIndex: 240 },
];
const MOVING_SECONDS = 4_040;

function descentMetres(points: Position[]): number {
  let descent = 0;
  for (let index = 1; index < points.length; index++) {
    const drop = (points[index - 1]?.[2] ?? 0) - (points[index]?.[2] ?? 0);
    if (drop > 0) {
      descent += drop;
    }
  }

  return descent;
}

/** The strip's own frame: the map above it, so a variant is read where it sits. */
function Stage({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-[520px] flex-col overflow-hidden rounded-2xl bg-[var(--base)] shadow-[var(--shadow)]">
      <div className="grid flex-1 place-items-center bg-[repeating-linear-gradient(45deg,color-mix(in_oklab,var(--ink-2)_6%,transparent)_0_12px,transparent_12px_24px)] text-[var(--ink-2)] text-sm">
        the map
      </div>
      {children}
    </div>
  );
}

function Strip({ children }: { children: ReactNode }) {
  return (
    <section
      aria-label="Planned route"
      className="border-[var(--rule)] border-t bg-[var(--panel)] px-4 py-2"
    >
      {children}
    </section>
  );
}

function Figure({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <span className="flex items-baseline gap-1 text-[var(--ink-2)]">
      {icon}
      <span className="font-medium text-[var(--ink)] tabular-nums">{children}</span>
    </span>
  );
}

function useMeasured() {
  return useMemo(() => {
    const profile = buildProfile(LINE);
    const surface = summariseSurface(LINE, RANGES);
    const shares = gradientSharesBySign(LINE);
    const total = profile?.totalDistanceMetres ?? 0;

    return {
      profile,
      surface,
      total,
      descent: descentMetres(LINE),
      ascent: total === 0 ? 0 : 119,
      steepness: steepnessEntries(shares, total).filter((entry) => entry.share > 0.001),
      ground: groundSegments(surface),
      mix: surfaceEntries(surface),
    };
  }, []);
}

/* ── A. One line ──────────────────────────────────────────────────────────── */

function VariantA() {
  const { profile, total, ascent, descent } = useMeasured();
  const [open, setOpen] = useState(true);
  const [active, setActive] = useState<number | null>(null);

  return (
    <Stage>
      <Strip>
        <div className="flex items-center justify-between gap-3">
          <output aria-label="Planned route summary" className="flex items-center gap-4 text-sm">
            <span className="font-semibold text-base tabular-nums">{formatDistance(total)}</span>
            <Figure icon={<IconArrowUpRight size={14} stroke={1.8} aria-hidden="true" />}>
              {formatAscent(ascent)}
            </Figure>
            <Figure icon={<IconArrowDownRight size={14} stroke={1.8} aria-hidden="true" />}>
              {formatAscent(descent)}
            </Figure>
            <Figure icon={<IconClock size={14} stroke={1.8} aria-hidden="true" />}>
              {formatDuration(MOVING_SECONDS)}
            </Figure>
            <span className="text-[var(--ink-2)] text-xs tabular-nums">
              {formatElevation(profile?.minElevationMetres ?? 0)}–
              {formatElevation(profile?.maxElevationMetres ?? 0)}
            </span>
          </output>
          <Button
            variant="ghost"
            icon={
              open ? <IconLayoutBottombarCollapse stroke={1.6} /> : <IconMountain stroke={1.6} />
            }
            aria-expanded={open}
            onClick={() => setOpen(!open)}
          >
            {open ? "Hide" : "Show"}
          </Button>
        </div>
        {open ? (
          <ElevationProfile
            title="Planned route elevation"
            profile={profile}
            activeMetres={active}
            onActiveChange={setActive}
          />
        ) : null}
      </Strip>
    </Stage>
  );
}

/* ── B. Line, ribbon and mix ──────────────────────────────────────────────── */

function VariantB() {
  const { profile, surface, total, ascent, descent, ground, steepness } = useMeasured();
  const [active, setActive] = useState<number | null>(null);

  return (
    <Stage>
      <Strip>
        <div className="flex items-center justify-between gap-3">
          <output aria-label="Planned route summary" className="flex items-center gap-4 text-sm">
            <span className="font-semibold text-base tabular-nums">{formatDistance(total)}</span>
            <Figure icon={<IconArrowUpRight size={14} stroke={1.8} aria-hidden="true" />}>
              {formatAscent(ascent)}
            </Figure>
            <Figure icon={<IconArrowDownRight size={14} stroke={1.8} aria-hidden="true" />}>
              {formatAscent(descent)}
            </Figure>
            <Figure icon={<IconClock size={14} stroke={1.8} aria-hidden="true" />}>
              {formatDuration(MOVING_SECONDS)}
            </Figure>
          </output>
          <Button
            variant="ghost"
            icon={<IconLayoutBottombarCollapse stroke={1.6} />}
            aria-expanded
            aria-label="Hide the route detail"
          >
            Hide
          </Button>
        </div>
        <ElevationProfile
          title="Planned route elevation"
          profile={profile}
          activeMetres={active}
          onActiveChange={setActive}
        />
        <div className="flex flex-col gap-1.5 pt-1">
          <GroundRibbon
            segments={ground}
            surface={surface}
            thin
            highlight={null}
            onHighlightChange={() => {}}
          />
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[var(--ink-2)] text-xs tabular-nums">
            {steepness.map((entry) => (
              <span key={entry.label} className="flex items-center gap-1">
                <span
                  aria-hidden="true"
                  className="size-2 rounded-[3px]"
                  style={{ background: `var(${entry.colour})` }}
                />
                {entry.label} {formatDistance(entry.metres)}
              </span>
            ))}
          </div>
        </div>
      </Strip>
    </Stage>
  );
}

/* ── C. Stops on one rail ─────────────────────────────────────────────────── */

const STOPS = [
  { key: "profile", label: "Profile", icon: <IconMountain size={14} stroke={1.8} /> },
  { key: "ground", label: "Ground", icon: <IconRoad size={14} stroke={1.8} /> },
] as const;

function VariantC() {
  const { profile, surface, total, ascent, descent, ground, mix, steepness } = useMeasured();
  const [stop, setStop] = useState<"profile" | "ground">("profile");
  const [active, setActive] = useState<number | null>(null);

  return (
    <Stage>
      <Strip>
        <Tabs.Root value={stop} onValueChange={(next) => setStop(next as "profile" | "ground")}>
          <div className="flex items-center justify-between gap-3">
            <output aria-label="Planned route summary" className="flex items-center gap-4 text-sm">
              <span className="font-semibold text-base tabular-nums">{formatDistance(total)}</span>
              <Figure icon={<IconArrowUpRight size={14} stroke={1.8} aria-hidden="true" />}>
                {formatAscent(ascent)}
              </Figure>
              <Figure icon={<IconArrowDownRight size={14} stroke={1.8} aria-hidden="true" />}>
                {formatAscent(descent)}
              </Figure>
              <Figure icon={<IconClock size={14} stroke={1.8} aria-hidden="true" />}>
                {formatDuration(MOVING_SECONDS)}
              </Figure>
            </output>
            <div className="flex items-center gap-2">
              <SegmentedTrack active={stop}>
                <Tabs.List className="contents">
                  {STOPS.map((item) => (
                    <Tabs.Tab
                      key={item.key}
                      value={item.key}
                      data-segment={item.key}
                      className={segmentClass("sm")}
                    >
                      {item.icon}
                      <SegmentLabel>{item.label}</SegmentLabel>
                    </Tabs.Tab>
                  ))}
                </Tabs.List>
              </SegmentedTrack>
              <Button
                variant="ghost"
                icon={<IconLayoutBottombarCollapse stroke={1.6} />}
                aria-expanded
                aria-label="Hide the route detail"
              >
                Hide
              </Button>
            </div>
          </div>
          <Tabs.Panel value="profile">
            <ElevationProfile
              title="Planned route elevation"
              profile={profile}
              activeMetres={active}
              onActiveChange={setActive}
            />
          </Tabs.Panel>
          <Tabs.Panel value="ground" className="flex flex-col gap-3 py-3">
            <GroundRibbon
              segments={ground}
              surface={surface}
              highlight={null}
              onHighlightChange={() => {}}
            />
            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm tabular-nums">
              {[...mix, ...steepness].map((entry) => (
                <span key={entry.label} className="flex items-center gap-2">
                  <span
                    aria-hidden="true"
                    className="size-2.5 shrink-0 rounded-[3px]"
                    style={{ background: `var(${entry.colour})` }}
                  />
                  <span className="flex-1 truncate text-[var(--ink-2)]">{entry.label}</span>
                  <span>{formatDistance(entry.metres)}</span>
                </span>
              ))}
            </div>
          </Tabs.Panel>
        </Tabs.Root>
      </Strip>
    </Stage>
  );
}

const meta = {
  title: "Spikes/Planner Dock",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const A_Line: Story = { name: "A · One line", render: () => <VariantA /> };
export const B_Ground: Story = { name: "B · Line, ribbon and mix", render: () => <VariantB /> };
export const C_Stops: Story = { name: "C · Stops on one rail", render: () => <VariantC /> };
