import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity, Fitness, FitnessDay, FitnessScaleOutlook, Status } from "../../api/types";
import { FitnessPage } from "./FitnessPage";

function day(date: string, overrides: Partial<FitnessDay> = {}): FitnessDay {
  return {
    date,
    trimpLoad: 40,
    trimpFitness: 30,
    trimpFatigue: 45,
    trimpForm: -15,
    tssLoad: 80,
    tssFitness: 60,
    tssFatigue: 90,
    tssForm: -30,
    ...overrides,
  };
}

const STATUS: Status = {
  ready: true,
  converged: true,
  targets: [],
  sync: {
    state: "idle",
    sourceRoutes: 0,
    created: 0,
    updated: 0,
    deleted: 0,
    phases: {},
    surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
  },
};

const TIMELINE: Fitness = {
  days: [day("2026-08-22"), day("2026-08-23"), day("2026-08-24")],
  weeks: [{ weekStart: "2026-08-17", zoneSeconds: [600, 1200, 300, 0, 0] }],
};

/**
 * The page asks for a window measured from today, so its query key is not one a
 * test can seed. The answer is stubbed at the transport instead.
 */
function show(fitness: Fitness = TIMELINE, activities: Activity[] = []) {
  // Routed by URL: the page's own chrome polls the status, and answering that
  // with a timeline is what makes the menu bar throw rather than the page fail.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      // Fitness first: the activities path is a prefix of it.
      const body = url.includes("/v1/activities/fitness")
        ? fitness
        : url.includes("/v1/activities")
          ? { activities }
          : STATUS;

      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // PageShell's own chrome reads these; the timeline is what the stub answers.
  client.setQueryData(statusQuery().queryKey, STATUS);
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin: false },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <FitnessPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  nextRideID = 1;
});

function scaleOutlook(fitness: number): FitnessScaleOutlook {
  const days = Array.from({ length: 21 }, (_, index) => ({
    date: `2026-08-${String(25 + index).padStart(2, "0")}`,
    fitness,
    fatigue: fitness,
    form: 0,
  }));

  return {
    rampPerWeek: 3,
    habitualDailyLoad: 50,
    weekLoadLow: 412,
    weekLoadHigh: 488,
    plans: [
      {
        plan: "rest",
        dailyLoad: 0,
        days: days.map((one, index) => ({ ...one, form: index * 4 - 20 })),
      },
      { plan: "habitual", dailyLoad: 50, days },
      { plan: "build", dailyLoad: 60, days },
    ],
  };
}

const OUTLOOK: Fitness["outlook"] = {
  date: "2026-08-24",
  tss: scaleOutlook(60),
  trimp: { ...scaleOutlook(30), weekLoadLow: 200, weekLoadHigh: 240 },
};

describe("FitnessPage", () => {
  it("offers a range and both scales", async () => {
    show();

    expect(await screen.findByRole("group", { name: "Range" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "6 months" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Scale" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "TRIMP" })).toBeInTheDocument();
  });

  it("names the scale the chart is on, and changes it with the toggle", async () => {
    show();

    expect(await screen.findByRole("img", { name: /stress score scale/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "TRIMP" }));
    expect(await screen.findByRole("img", { name: /TRIMP scale/ })).toBeInTheDocument();
  });

  it("leads with fitness and form as figures", async () => {
    show();

    expect(await screen.findByRole("group", { name: "Fitness" })).toHaveTextContent("60");
    // -30 of 60 is half of fitness below, which is the high-risk band.
    expect(screen.getByRole("group", { name: "Form" })).toHaveTextContent(
      "−50%of fitnessHigh risk",
    );
  });

  it("reads the outlook on the scale the page is on", async () => {
    show({ ...TIMELINE, outlook: OUTLOOK });

    expect(
      await screen.findByRole("group", { name: "Next 7 days, to keep building" }),
    ).toHaveTextContent("412–488");
    expect(screen.getByRole("group", { name: "Ramp rate" })).toHaveTextContent("+3.0");
    // The rest plan's form reaches the fresh band on its seventh day.
    expect(screen.getByText(/fresh after 7 days of rest/)).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: /and 21 days projected under 3 plans/ }),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "TRIMP" }));
    expect(await screen.findByText("200–240")).toBeInTheDocument();
  });

  // Projected from another day, the plans would not join the line they continue.
  it("leaves out an outlook projected from a day other than the last one shown", async () => {
    show({ ...TIMELINE, outlook: { ...OUTLOOK, date: "2026-08-20" } });

    await screen.findByRole("group", { name: "Scale" });
    expect(screen.queryByRole("group", { name: "Ramp rate" })).toBeNull();
    expect(screen.queryByText("412–488")).toBeNull();
  });

  it("draws each week's time in zone", async () => {
    show();

    expect(await screen.findByRole("heading", { name: "Time in zone" })).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: "Hours in each heart-rate zone over 1 week" }),
    ).toBeInTheDocument();
  });

  // The timeline's last day is a Monday, so its week has begun but has no width on the axis yet.
  it("keeps the week begun on the last day served", async () => {
    show({
      ...TIMELINE,
      weeks: [...TIMELINE.weeks, { weekStart: "2026-08-24", zoneSeconds: [0, 900, 0, 0, 0] }],
    });

    expect(
      await screen.findByRole("img", { name: "Hours in each heart-rate zone over 2 weeks" }),
    ).toBeInTheDocument();
  });

  // A rider whose rides have not been derived is told what is missing, not shown
  // an empty chart.
  it("says what is missing when nothing has been worked out", async () => {
    show({ days: [], weeks: [] });

    expect(await screen.findByText(/Nothing has been worked out yet/)).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Scale" })).not.toBeInTheDocument();
  });
});

/** One ride carrying whatever decoupling the test is about. */
let nextRideID = 1;

function ride(startedAt: string, decouplingPercent?: number): Activity {
  return {
    id: String(nextRideID++),
    startedAt,
    distanceMetres: 40_000,
    movingSeconds: 5400,
    elapsedSeconds: 5700,
    ascentMetres: 400,
    typeId: 15,
    locationId: 1,
    provider: "wahoo",
    ...(decouplingPercent === undefined ? {} : { metrics: { decouplingPercent } }),
  };
}

describe("FitnessPage decoupling", () => {
  it("draws the rides in the range that carry one", async () => {
    show(TIMELINE, [
      ride("2026-08-23T08:00:00Z", 4.2),
      ride("2026-08-23T17:00:00Z", 7.9),
      ride("2026-06-01T08:00:00Z", 3),
    ]);

    expect(await screen.findByRole("heading", { name: "Decoupling" })).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: /Aerobic decoupling over 2 rides/ }),
    ).toBeInTheDocument();
  });

  // A ride late in the evening in the service's zone is already the next day in UTC.
  it("places a ride on its day in the service's zone", async () => {
    show(TIMELINE, [ride("2026-08-21T22:30:00Z", 5)]);

    expect(
      await screen.findByRole("img", { name: /Aerobic decoupling over 1 ride/ }),
    ).toBeInTheDocument();
  });

  it("says the rides were not read rather than drawing an empty season", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/activities/fitness")) {
          return new Response(JSON.stringify(TIMELINE), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.includes("/v1/activities")) {
          return new Response("", { status: 503 });
        }

        return new Response(JSON.stringify(STATUS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(statusQuery().queryKey, STATUS);
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <FitnessPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The service did not say what has been ridden",
    );
  });

  it("draws nothing where no ride in the range carries one", async () => {
    show(TIMELINE, [ride("2026-08-23T08:00:00Z")]);

    await screen.findByRole("group", { name: "Scale" });
    expect(screen.queryByRole("heading", { name: "Decoupling" })).toBeNull();
  });
});

describe("FitnessPage power curve", () => {
  it("draws the curve and reads each duration out beside it", async () => {
    show({
      ...TIMELINE,
      powerCurve: [
        { seconds: 5, watts: 912 },
        { seconds: 1200, watts: 268.4 },
      ],
    });

    expect(await screen.findByRole("heading", { name: "Power duration" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Best mean power over 5s, 20m" })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: "20m 268 W" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Change" })).toBeNull();
  });

  it("sets the curve against the range before it where the service had one", async () => {
    show({
      ...TIMELINE,
      powerCurve: [
        { seconds: 5, watts: 912 },
        { seconds: 1200, watts: 268.4 },
      ],
      powerCurvePrevious: [{ seconds: 1200, watts: 260 }],
    });

    expect(await screen.findByRole("columnheader", { name: "Change" })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: "20m 268 W +8" })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: "5s 912 W —" })).toBeInTheDocument();
    expect(screen.getByText("the 6 months before")).toBeInTheDocument();
  });

  // The axis spans only the durations this range reached; an hour from the range before would fall off it.
  it("draws the range before only over the durations this range reached", async () => {
    const { container } = show({
      ...TIMELINE,
      powerCurve: [
        { seconds: 5, watts: 912 },
        { seconds: 1200, watts: 268.4 },
      ],
      powerCurvePrevious: [
        { seconds: 5, watts: 900 },
        { seconds: 1200, watts: 260 },
        { seconds: 3600, watts: 240 },
      ],
    });

    await screen.findByRole("heading", { name: "Power duration" });
    const previous = container.querySelector('polyline[stroke-dasharray="4 3"]');
    expect(previous?.getAttribute("points")?.split(" ")).toHaveLength(2);
  });

  it("draws nothing where no ride in the window carried a meter", async () => {
    show(TIMELINE);

    await screen.findByRole("group", { name: "Scale" });
    expect(screen.queryByRole("heading", { name: "Power duration" })).toBeNull();
  });
});
