import { describe, expect, it } from "vitest";
import { SYNC_PHASES } from "../api/types";
import type { SyncFailureCategory } from "./syncGuidance";
import { GUIDANCE_LABELS, SYNC_FAILURE_CATEGORIES, syncGuidance } from "./syncGuidance";

/** The two gates, which are the reason this module exists. */
const DELETION_GATES: SyncFailureCategory[] = ["empty_source", "deletion_limit"];

describe("syncGuidance", () => {
  it("says nothing about a run that succeeded", () => {
    expect(syncGuidance("source", "succeeded", undefined)).toBeUndefined();
    expect(syncGuidance("targets", "succeeded", "")).toBeUndefined();
  });

  it.each(SYNC_FAILURE_CATEGORIES)("explains %s and gives a next action", (category) => {
    const guidance = syncGuidance("targets", "failed", category);

    expect(guidance).toBeDefined();
    expect(guidance?.headline).not.toHaveLength(0);
    expect(guidance?.remediation).not.toHaveLength(0);
    // Guidance is prose, not the wire word dressed up. Several categories are
    // ordinary English — a course is a course — so the check that nothing is
    // echoed uses a value no sentence would contain; see the last case here.
    expect(guidance?.headline).not.toBe(category);
    expect(guidance?.remediation).not.toBe(category);
  });

  it.each(DELETION_GATES)("reads %s as a gate holding, not as a fault", (category) => {
    const guidance = syncGuidance("targets", "blocked", category);

    expect(guidance?.kind).toBe("blocked");
    expect(guidance?.remediation).toContain("Nothing was deleted");
  });

  it("keeps a gate readable as a gate whatever result word carries it", () => {
    // A gate is about what the target now holds, and that is the same whether
    // the run recorded itself as blocked or as failed.
    expect(syncGuidance("targets", "failed", "deletion_limit")?.kind).toBe("blocked");
    expect(syncGuidance("targets", "blocked", "deletion_limit")?.kind).toBe("blocked");
  });

  it("distinguishes a blocked run from a failed one", () => {
    const blocked = syncGuidance("targets", "blocked", "empty_source");
    const failed = syncGuidance("targets", "failed", "destination");

    expect(blocked?.kind).toBe("blocked");
    expect(failed?.kind).toBe("failed");
    expect(GUIDANCE_LABELS[blocked?.kind ?? "failed"]).not.toBe(
      GUIDANCE_LABELS[failed?.kind ?? "failed"],
    );
  });

  it.each(SYNC_PHASES)("names the %s half in the headline", (phase) => {
    const guidance = syncGuidance(phase, "failed", "state");
    const other = syncGuidance(phase === "source" ? "targets" : "source", "failed", "state");

    expect(guidance?.headline).not.toBe(other?.headline);
    expect(guidance?.remediation).toBe(other?.remediation);
  });

  it("reports a run that never started as waiting rather than as a failure", () => {
    expect(syncGuidance("targets", "not_ready", "")?.kind).toBe("waiting");
    expect(syncGuidance("source", "skipped", undefined)?.kind).toBe("waiting");
  });

  it("asks the operator to look when the category is one it does not know", () => {
    const guidance = syncGuidance("source", "failed", "a_category_from_a_later_service");

    expect(guidance?.kind).toBe("failed");
    // Whatever the unknown word was, it must not be echoed back onto the page.
    expect(guidance?.headline).not.toContain("a_category_from_a_later_service");
    expect(guidance?.remediation).not.toContain("a_category_from_a_later_service");
  });

  it("treats a failure with no category as one it cannot explain", () => {
    expect(syncGuidance("source", "failed", undefined)?.kind).toBe("failed");
    expect(syncGuidance("source", "failed", "")?.kind).toBe("failed");
  });

  /**
   * The guidance table is constant, so nothing a run carries can reach the page
   * through it. Feeding every input a hostile value and finding none of them in
   * the output is what keeps that true as the table grows.
   */
  it("never echoes what the service reported into its guidance", () => {
    const secret = "Ventoux-via-Bédoin";

    for (const phase of SYNC_PHASES) {
      const guidance = syncGuidance(phase, secret, secret);

      expect(guidance?.headline).not.toContain(secret);
      expect(guidance?.remediation).not.toContain(secret);
    }
  });
});
