import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useState } from "react";
import { riderProfileQuery } from "../../api/queries";
import type { RiderProfile } from "../../api/types";
import { WahooDeviceCard } from "./WahooDeviceCard";

function Seeded({ wahoo }: { wahoo: RiderProfile["wahoo"] }): ReactNode {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(riderProfileQuery().queryKey, {
      profile: {},
      suggestions: {},
      zwift: { emailSet: false, passwordSet: false },
      wahoo,
    } satisfies RiderProfile);

    return next;
  });

  return (
    <QueryClientProvider client={client}>
      <div className="max-w-2xl p-4">
        <WahooDeviceCard />
      </div>
    </QueryClientProvider>
  );
}

const meta = {
  title: "Settings/WahooDeviceCard",
  component: WahooDeviceCard,
} satisfies Meta<typeof WahooDeviceCard>;

export default meta;

type Story = StoryObj<typeof meta>;

export const NotConnected: Story = {
  render: () => <Seeded wahoo={{ emailSet: false, passwordSet: false, signInRefused: false }} />,
};

export const Connected: Story = {
  render: () => <Seeded wahoo={{ emailSet: true, passwordSet: true, signInRefused: false }} />,
};

export const Refused: Story = {
  render: () => <Seeded wahoo={{ emailSet: true, passwordSet: true, signInRefused: true }} />,
};
