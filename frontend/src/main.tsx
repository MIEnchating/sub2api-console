import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { router } from "./router";
import { Toaster } from "./components/ui/sonner";
import { TooltipProvider } from "./components/ui/tooltip";
import { createConsoleQueryClient } from "./lib/query-client";
import "./styles.css";
import "@fontsource-variable/public-sans";

const queryClient = createConsoleQueryClient();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
        <Toaster closeButton duration={5000} position="top-center" richColors />
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
);
