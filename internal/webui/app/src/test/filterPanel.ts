import { screen, waitFor } from "@testing-library/react";

/**
 * Focuses one thumb of the open filter panel. The popover moves focus into
 * itself a tick after opening, so a thumb focused before that is lost.
 */
export async function focusThumb(name: string): Promise<void> {
  await waitFor(() => {
    const panel = screen.getByRole("dialog", { name: "Library filters" });
    if (!panel.contains(document.activeElement)) {
      throw new Error("filter panel has not taken focus yet");
    }
  });
  screen.getByRole("slider", { name }).focus();
}
