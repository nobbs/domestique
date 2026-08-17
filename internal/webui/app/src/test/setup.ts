import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library only registers its own cleanup when Vitest globals are on.
// This suite imports its helpers explicitly, so unmount between tests here or
// each render leaks into the next one's queries.
afterEach(cleanup);
