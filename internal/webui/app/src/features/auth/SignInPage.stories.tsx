import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { expect } from "storybook/test";
import { SignInPage } from "./SignInPage";

// The page reads its refusal out of the address, so each story is the address
// the service redirected the browser to.
function at(search: string) {
  return (Story: () => React.ReactNode) => (
    <MemoryRouter initialEntries={[`/auth/login${search}`]}>
      <Story />
    </MemoryRouter>
  );
}

const meta = {
  title: "Features/Sign In Page",
  component: SignInPage,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SignInPage>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * A first arrival: the name, and the one control. The form posts the document
 * rather than fetching, because the answer is a redirect out to the provider.
 */
export const Default: Story = {
  decorators: [at("")],
  play: async ({ canvas }) => {
    const button = await canvas.findByRole("button", { name: "Sign in" });

    await expect(button).toHaveAttribute("type", "submit");
    await expect(button.closest("form")).toHaveAttribute("action", "/auth/start");
    await expect(canvas.queryByRole("alert")).toBeNull();
  },
};

/** A subject the allowlist does not name, told apart from a service that failed. */
export const NotAllowed: Story = {
  decorators: [at("?error=not_allowed")],
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "This account is not allowed to sign in.",
    );
  },
};

/** Every other step that can fail, saying the same thing about each of them. */
export const Failed: Story = {
  decorators: [at("?error=failed")],
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "Sign-in could not be completed.",
    );
  },
};
