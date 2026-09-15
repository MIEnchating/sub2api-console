import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "../../../src/components/ui/sonner";
import { BrowserLogin } from "../../../src/features/upstreams/components/browser-login/browser-login";

const client = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});
createRoot(document.getElementById("browser-root")!).render(
  <QueryClientProvider client={client}>
    <BrowserLogin host="login.example.test" />
    <Toaster closeButton duration={5000} position="top-center" richColors />
  </QueryClientProvider>,
);
