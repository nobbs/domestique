import type { Preview } from "@storybook/react-vite";
import { type ReactNode, useEffect } from "react";
import "../src/index.css";

function StorybookTheme({ children, theme }: { children: ReactNode; theme: unknown }) {
  useEffect(() => {
    document.documentElement.dataset.theme = theme === "dark" ? "dark" : "light";
  }, [theme]);

  return <div className="min-h-dvh bg-background p-6 text-foreground">{children}</div>;
}

const preview = {
  globalTypes: {
    theme: {
      description: "Color theme",
      toolbar: {
        icon: "paintbrush",
        items: [
          { value: "light", title: "Light" },
          { value: "dark", title: "Dark" },
        ],
      },
    },
  },
  initialGlobals: { theme: "light" },
  decorators: [
    (Story, context) => (
      <StorybookTheme theme={context.globals.theme}>
        <Story />
      </StorybookTheme>
    ),
  ],
} satisfies Preview;

export default preview;
