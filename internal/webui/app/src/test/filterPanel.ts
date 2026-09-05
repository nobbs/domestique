import { screen, waitFor } from "@testing-library/react";

/**
 * Focuses one thumb of the open filter panel. The popover moves focus to its
 * first control a tick after opening, so a thumb focused before that is lost.
 */
export async function focusThumb(name: string): Promise<void> {
  await waitFor(() => {
    if (document.activeElement?.getAttribute("aria-label") !== "Distance min") {
      throw new Error("filter panel has not taken focus yet");
    }
  });
  screen.getByRole("slider", { name }).focus();
}
