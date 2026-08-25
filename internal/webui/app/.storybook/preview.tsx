import type { Preview } from "@storybook/react-vite";
import "../src/index.css";

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
    (Story, context) => {
      document.documentElement.dataset.theme = context.globals.theme === "dark" ? "dark" : "light";

      return (
        <div className="min-h-dvh bg-background p-6 text-foreground">
          <Story />
        </div>
      );
    },
  ],
} satisfies Preview;

export default preview;
