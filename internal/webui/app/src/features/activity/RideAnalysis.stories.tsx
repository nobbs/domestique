import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fitnessQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity } from "../../api/types";
import { fitnessWindowFor, RideAnalysis } from "./RideAnalysis";

const RIDE: Activity = {
  id: "1",
  startedAt: "2026-09-01T06:00:00Z",
  distanceMetres: 36_200,
  movingSeconds: 5_220,
  elapsedSeconds: 5_460,
  ascentMetres: 180,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
  metrics: { powerTss: 64, trimp: 64, estimatedPowerWatts: 154 },
  analysis: {
    text: "A 36.2 km outdoor ride, held at an easy aerobic pace.",
    model: "claude-sonnet-5",
    promptRevision: 3,
    analysedAt: "2026-09-22T14:07:00Z",
    document: {
      rideType: "intervals",
      headline: "Six on-target intervals, the last one deep into zone 5",
      summary:
        "A 36.2 km outdoor ride, 87 minutes moving, held at an easy aerobic pace with average heart rate of 113 bpm and 92% of moving time in zones 1-2. A brief surge around the 17 km mark took heart rate to a peak of 155 bpm for roughly a minute before settling back down. No power meter was fitted; estimated pedalling power averaged 154 W across the 92% of the ride spent pedalling. Conditions were mild and breezy with drizzle noted at the start but no measurable rainfall.",
      loadEffect:
        "TSS and TRIMP of 64 kept fitness essentially flat at 42 while nudging fatigue up slightly, leaving form at -2, a low-cost day close to the rider's habitual daily load of around 42.",
      highlights: [
        "Six intervals at 270-285 W landed within 1% of target power each time",
        "Cadence averaged 92 rpm, spinning smoothly at high power",
        "Final effort pushed heart rate to 167 bpm, deep into zone 5",
      ],
      concerns: [],
      nextSession: {
        advice:
          "Form is close to neutral, a good window to bring back a short structured zone 3-4 interval session rather than another pure endurance ride.",
        suggestedRestDays: 0,
      },
      dataGaps: ["No temperature reading recorded for the entire ride"],
    },
  },
};

const meta = {
  title: "Features/Activity/Analysis",
  component: RideAnalysis,
  tags: ["autodocs"],
  decorators: [
    (Story) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
      });
      client.setQueryData(webUIConfigQuery().queryKey, {
        basemaps: [],
        sourceBaseUrls: {},
        timezone: "UTC",
        identity: { display: "rider@example.test", admin: true },
      });
      client.setQueryData(tasksQuery().queryKey, {
        tasks: [{ name: "activity:reanalyse", scheduled: false, enabled: true, running: 0 }],
      });
      client.setQueryData(fitnessQuery(fitnessWindowFor("2026-09-01")).queryKey, {
        days: [
          {
            date: "2026-09-01",
            trimpLoad: 64,
            trimpFitness: 42,
            trimpFatigue: 44,
            trimpForm: -2,
            tssLoad: 64,
            tssFitness: 42,
            tssFatigue: 44,
            tssForm: -2,
          },
        ],
        weeks: [],
      });
      return (
        <QueryClientProvider client={client}>
          <div className="max-w-2xl p-6">
            <Story />
          </div>
        </QueryClientProvider>
      );
    },
  ],
} satisfies Meta<typeof RideAnalysis>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The card-per-field document, its lead sentence emphasised, form read from the fitness timeline. */
export const Document: Story = {
  args: { ride: RIDE },
};
