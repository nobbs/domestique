/**
 * The colour scheme, at the end of the bar beside the session.
 *
 * A cycle rather than a menu: there are three choices and no reason to open
 * anything to reach the next one. What it costs is that the glyph alone cannot
 * say where a press goes, so the name says both — the scheme in force and the
 * one a press would choose — and a reader who cannot see the glyph is told the
 * same thing a sighted one infers from it.
 */

import { IconDeviceDesktop, IconMoon, IconSun } from "@tabler/icons-react";
import type { ThemeChoice } from "../lib/theme";
import { nextThemeChoice, useThemeChoice } from "../lib/theme";
import { Button } from "./Button";

const GLYPHS: Record<ThemeChoice, typeof IconSun> = {
  system: IconDeviceDesktop,
  light: IconSun,
  dark: IconMoon,
};

/** Lower case because these are read inside a sentence, not as a menu's items. */
const NAMES: Record<ThemeChoice, string> = {
  system: "system",
  light: "light",
  dark: "dark",
};

export function ThemeToggle() {
  const [choice, setChoice] = useThemeChoice();
  const next = nextThemeChoice(choice);
  const Glyph = GLYPHS[choice];
  const name = `Theme: ${NAMES[choice]}. Switch to ${NAMES[next]}.`;

  return (
    <Button
      aria-label={name}
      className="size-8 shrink-0 rounded-full p-0"
      icon={<Glyph stroke={1.6} />}
      onClick={() => setChoice(next)}
      title={name}
      variant="ghost"
    />
  );
}
