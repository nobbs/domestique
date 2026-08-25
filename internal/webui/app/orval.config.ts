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
      // A target identifier is operator-configured free text, so a reserved
      // character in one must travel as a single escaped path segment rather
      // than silently addressing a different route.
      urlEncodeParameters: true,
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
