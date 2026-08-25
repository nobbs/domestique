import { defineConfig } from "orval";

export default defineConfig({
  domestique: {
    input: {
      target: "../../../api/openapi.yaml",
      filters: { tags: ["browser-api"] },
    },
    output: {
      client: "react-query",
      target: "./src/api/generated.ts",
      mock: false,
      override: {
        fetch: { forceSuccessResponse: true },
        mutator: { name: "domestiqueRequest", path: "./src/api/request.ts" },
        operations: {
          getSyncRuns: { query: { useInfinite: true, useInfiniteQueryParam: "after" } },
        },
      },
    },
  },
});
