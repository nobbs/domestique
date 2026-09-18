import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getGetPlanDeliveryQueryKey, type PlanTargetDelivery } from "../../api/generated";
import { PlanDeliveryTrigger } from "./PlanDelivery";

const PLAN_ID = 4;

function client(targets: PlanTargetDelivery[]) {
  const next = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  next.setQueryData(getGetPlanDeliveryQueryKey(PLAN_ID), { data: { targets } });

  return next;
}

const meta = {
  title: "Features/Planner/Delivery",
  component: PlanDeliveryTrigger,
  decorators: [
    (Story) => (
      <div className="flex justify-end p-4">
        <Story />
      </div>
    ),
  ],
  args: { planId: PLAN_ID, published: true },
} satisfies Meta<typeof PlanDeliveryTrigger>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllCurrent: Story = {
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={client([
          {
            id: "t1",
            own: true,
            ownerNickname: "Alexej",
            state: "current",
            deliveredAt: "2026-09-18T09:00:00Z",
          },
          {
            id: "t2",
            own: false,
            ownerNickname: "Marta",
            state: "current",
            deliveredAt: "2026-09-18T08:45:00Z",
          },
        ])}
      >
        <Story />
      </QueryClientProvider>
    ),
  ],
};

export const PushInFlight: Story = {
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={client([
          {
            id: "t1",
            own: true,
            ownerNickname: "Alexej",
            state: "current",
            deliveredAt: "2026-09-16T07:00:00Z",
          },
          { id: "t2", own: false, ownerNickname: "Marta", state: "pending" },
        ])}
      >
        <Story />
      </QueryClientProvider>
    ),
  ],
};

export const Failures: Story = {
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={client([
          {
            id: "t1",
            own: true,
            ownerNickname: "Alexej",
            state: "current",
            deliveredAt: "2026-09-18T09:00:00Z",
          },
          {
            id: "t2",
            own: false,
            ownerNickname: "Marta",
            state: "failed",
            failure: "authorization",
            deliveredAt: "2026-09-16T07:00:00Z",
          },
          { id: "t3", own: false, ownerNickname: "Finn", state: "failed", failure: "destination" },
        ])}
      >
        <Story />
      </QueryClientProvider>
    ),
  ],
};

export const Draft: Story = {
  args: { published: false },
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={client([
          { id: "t1", own: true, ownerNickname: "Alexej", state: "absent" },
          { id: "t2", own: false, ownerNickname: "Marta", state: "absent" },
        ])}
      >
        <Story />
      </QueryClientProvider>
    ),
  ],
};

export const Removing: Story = {
  args: { published: false },
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={client([
          {
            id: "t1",
            own: true,
            ownerNickname: "Alexej",
            state: "pending",
            deliveredAt: "2026-09-16T07:00:00Z",
          },
          {
            id: "t2",
            own: false,
            ownerNickname: "Marta",
            state: "pending",
            deliveredAt: "2026-09-16T07:00:00Z",
          },
        ])}
      >
        <Story />
      </QueryClientProvider>
    ),
  ],
};
