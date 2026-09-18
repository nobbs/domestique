import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import { TooltipProvider } from "./components/ui/tooltip";
import "./index.css";
import { lockPageZoom } from "./lib/pageZoom";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// Document-wide and for the life of the tab, so there is nothing to undo.
lockPageZoom();

const container = document.getElementById("root");
if (!container) {
  throw new Error("the application root element is missing from the document");
}

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* One short hover delay for every tooltip; Base UI's own 600 ms reads as unresponsive. */}
      <TooltipProvider delay={150}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
);
