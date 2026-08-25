# Toggle group migration

## Component

`src/components/RouteKey.tsx`

## Replaced

`radix-ui` `ToggleGroup.Root` and `ToggleGroup.Item`

## New dependencies

`@base-ui/react` through the generated shadcn `ToggleGroup` source.

## Manual follow-up

None. The existing tests cover arrows, activation, and selecting an active item again to clear it.
