import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useState } from "react";
import { riderProfileQuery } from "../../api/queries";
import type { RiderProfile } from "../../api/types";
import { ZwiftAccountCard } from "./ZwiftAccountCard";

function Seeded({ zwift }: { zwift: RiderProfile["zwift"] }): ReactNode {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(riderProfileQuery().queryKey, {
      profile: {},
      suggestions: {},
      zwift,
    } satisfies RiderProfile);

    return next;
  });

  return (
    <QueryClientProvider client={client}>
      <div className="max-w-2xl p-4">
        <ZwiftAccountCard />
      </div>
    </QueryClientProvider>
  );
}

const meta = {
  title: "Settings/ZwiftAccountCard",
  component: ZwiftAccountCard,
} satisfies Meta<typeof ZwiftAccountCard>;

export default meta;

type Story = StoryObj<typeof meta>;

export const NotConnected: Story = {
  render: () => <Seeded zwift={{ emailSet: false, passwordSet: false }} />,
};

export const Connected: Story = {
  render: () => <Seeded zwift={{ emailSet: true, passwordSet: true }} />,
};
