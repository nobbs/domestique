import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fitnessQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import type {
  Activity,
  ActivityAnalysisDocument,
  Fitness,
  FitnessDay,
  TaskList,
  WebUIConfig,
} from "../../api/types";
import { calendarDay } from "../fitness/DecouplingPanel";
import { firstSentence, fitnessWindowFor, RideAnalysis } from "./RideAnalysis";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ride: Activity = {
  id: "1",
  startedAt: "2026-09-01T06:00:00Z",
  distanceMetres: 36000,
  movingSeconds: 3600,
  elapsedSeconds: 4000,
  ascentMetres: 420,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
};

const analysed: Activity = {
  ...ride,
  metrics: { trimp: 42 },
  analysis: {
    text: "A steady endurance ride.\n\nKeep tomorrow easy.",
    model: "claude-sonnet-5",
    promptRevision: 1,
    analysedAt: "2026-09-01T08:00:00Z",
  },
};

const withDocument: Activity = {
  ...ride,
  metrics: { powerTss: 64, trimp: 64 },
  analysis: {
    text: "A steady endurance ride.",
    model: "claude-sonnet-5",
    promptRevision: 3,
    analysedAt: "2026-09-01T08:00:00Z",
    document: {
      rideType: "endurance",
      headline: "Easy endurance spin with one short hard surge",
      summary:
        "A 36.2 km outdoor ride, 87 minutes moving, held at an easy aerobic pace with average heart rate of 113 bpm and 92% of moving time in zones 1-2. A brief surge around the 17 km mark took heart rate to a peak of 155 bpm for roughly a minute before settling back down. No power meter was fitted; estimated pedalling power averaged 154 W across the 92% of the ride spent pedalling. Conditions were mild and breezy with drizzle noted at the start but no measurable rainfall.",
      loadEffect:
        "TSS and TRIMP of 64 kept fitness essentially flat at 42 while nudging fatigue up slightly, leaving form at -2, a low-cost day close to the rider's habitual daily load of around 42.",
      highlights: [
        "92% of time spent in heart-rate zones 1-2",
        "Brief peak of 155 bpm near the 17 km mark",
        "Estimated power 154 W while pedalling, about 62% of FTP",
      ],
      concerns: [],
      nextSession: {
        advice:
          "Form is close to neutral, a good window to bring back a short structured zone 3-4 interval session rather than another pure endurance ride.",
        suggestedRestDays: 0,
      },
      dataGaps: ["No power meter fitted, only estimated power from speed and grade"],
    },
  },
};

function config(admin: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
  };
}

const withReanalyse: TaskList = {
  tasks: [{ name: "activity:reanalyse", scheduled: false, enabled: true, running: 0 }],
};

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

function fitnessData(days: FitnessDay[]): Fitness {
  return { days, weeks: [] };
}

/** `withDocument`, with the document replaced by `overrides` merged over its own. */
function withDocumentOverrides(overrides: Partial<ActivityAnalysisDocument>): Activity {
  const analysis = withDocument.analysis;
  const base = analysis?.document;
  if (!analysis || !base) {
    throw new Error("withDocument fixture has no document");
  }

  return {
    ...withDocument,
    analysis: { ...analysis, document: { ...base, ...overrides } },
  };
}

// Seeded by default so a test that does not care about the form figure never
// sends the suite's own network-refusing fetch a request; pass `null` to leave
// the query unseeded, for a test that stubs fetch itself to watch it being asked.
function show(
  value: Activity,
  admin = false,
  tasks: TaskList = withReanalyse,
  fitness: Fitness | null = fitnessData([]),
) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  client.setQueryData(webUIConfigQuery().queryKey, config(admin));
  client.setQueryData(tasksQuery().queryKey, tasks);
  if (fitness && value.startedAt) {
    const rideDay = calendarDay(value.startedAt, config(admin).timezone);
    client.setQueryData(fitnessQuery(fitnessWindowFor(rideDay)).queryKey, fitness);
  }

  return render(
    <QueryClientProvider client={client}>
      <RideAnalysis ride={value} />
    </QueryClientProvider>,
  );
}

describe("firstSentence", () => {
  it("splits at the first boundary followed by a capital", () => {
    expect(firstSentence("Short ride. Went well.")).toEqual({
      first: "Short ride.",
      rest: " Went well.",
    });
  });

  it("does not split on a decimal that happens to sit before a space", () => {
    expect(firstSentence("A 36.2 km ride went well.")).toEqual({
      first: "A 36.2 km ride went well.",
      rest: "",
    });
  });

  it("keeps the whole text as the first sentence when no boundary exists", () => {
    expect(firstSentence("no punctuation here")).toEqual({
      first: "no punctuation here",
      rest: "",
    });
  });
});

describe("RideAnalysis", () => {
  it("shows the text as written, with the model that wrote it", () => {
    show(analysed);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("A steady endurance ride.");
    expect(section).toHaveTextContent("Keep tomorrow easy.");
    expect(section).toHaveTextContent("claude-sonnet-5");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows nothing for a ride not analysed", () => {
    const { container } = show(ride);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders the headline, the emphasised first sentence, the three cards and the gaps line", async () => {
    show(withDocument, false, withReanalyse, fitnessData([day("2026-09-01", { tssForm: -2 })]));

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("Easy endurance spin with one short hard surge");
    expect(section).toHaveTextContent("endurance");

    // The lead sentence is bold; the rest of the summary is not.
    const lead = screen.getByText(
      "A 36.2 km outdoor ride, 87 minutes moving, held at an easy aerobic pace with average heart rate of 113 bpm and 92% of moving time in zones 1-2.",
    );
    expect(lead.tagName).toBe("SPAN");
    expect(lead).toHaveClass("font-medium");

    expect(section).toHaveTextContent(
      "Form is close to neutral, a good window to bring back a short structured zone 3-4 interval session",
    );
    expect(section).toHaveTextContent("no rest needed");

    // TSS and TRIMP each render their own figure, both 64 for this ride.
    expect(screen.getAllByText("64", { selector: "span" })).toHaveLength(2);
    expect(screen.getByText("TSS")).toBeInTheDocument();
    expect(screen.getByText("TRIMP")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("−2")).toBeInTheDocument());
    expect(section).toHaveTextContent("kept fitness essentially flat at 42");

    expect(section).toHaveTextContent("Nothing to flag");

    expect(section).toHaveTextContent("92% of time spent in heart-rate zones 1-2");
    expect(section).toHaveTextContent(
      "Not measured: No power meter fitted, only estimated power from speed and grade",
    );
    expect(section).toHaveTextContent("claude-sonnet-5");
  });

  it("shows the concern count and list when the document names any", () => {
    show(withDocumentOverrides({ concerns: ["Heart-rate strap covered only 16% of the ride"] }));

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("Heart-rate strap covered only 16% of the ride");
    expect(section).toHaveTextContent("1 concern");
    expect(section).not.toHaveTextContent("Nothing to flag");
  });

  it("falls back to the text when a document is absent", () => {
    show(analysed);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("A steady endurance ride.");
    expect(section).not.toHaveTextContent("Not measured");
  });

  it("omits highlights and the gaps line when empty", () => {
    show(withDocumentOverrides({ highlights: [], dataGaps: [] }));

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).not.toHaveTextContent("Highlights");
    expect(section).not.toHaveTextContent("Not measured");
  });

  it("shows a dash for form while the fitness query has no data, and never blocks on it", async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal("fetch", fetchMock);
    show(withDocument, false, withReanalyse, null);

    const section = screen.getByRole("region", { name: "Analysis" });
    expect(section).toHaveTextContent("Easy endurance spin");
    expect(screen.getAllByText("–").length).toBeGreaterThan(0);
    // Counts the request rather than letting it hang against the network: the
    // fitness query was asked for, but its never-resolving response never gates the panel.
    await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(0));
  });

  it("lets an admin ask again about an analysed ride", async () => {
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ status: "accepted" }), { status: 202 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    show(analysed, true);

    await userEvent.click(screen.getByRole("button", { name: "Analyse again" }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.some((call) => call[0] === "/v1/activities/1/reanalyse")).toBe(
        true,
      ),
    );
    expect(await screen.findByText(/Reload the page in a minute/)).toBeInTheDocument();
  });

  it("says so when the request is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_input: RequestInfo | URL, _init?: RequestInit) =>
          new Response(JSON.stringify({ error: "task_in_progress", message: "busy" }), {
            status: 409,
          }),
      ),
    );
    show(analysed, true);

    await userEvent.click(screen.getByRole("button", { name: "Analyse again" }));

    expect(await screen.findByText(/did not take the request/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Analyse again" })).toBeEnabled();
  });

  it("offers an admin a first analysis of a derived ride", () => {
    show({ ...ride, metrics: { trimp: 42 } }, true);

    expect(screen.getByRole("button", { name: "Analyse" })).toBeInTheDocument();
  });

  it("offers nothing where analysis is off or the ride is not derived", () => {
    const { container } = show({ ...ride, metrics: { trimp: 42 } }, true, { tasks: [] });
    expect(container).toBeEmptyDOMElement();

    const underived = show(ride, true);
    expect(underived.container).toBeEmptyDOMElement();
  });
});
