import { useEffect, useState, type ComponentProps } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SetupPage } from "../setup-page";

export function SetupTestPage(props: ComponentProps<typeof SetupPage>) {
  const [client] = useState(() => new QueryClient());
  useEffect(() => () => client.clear(), [client]);
  return (
    <QueryClientProvider client={client}>
      <SetupPage {...props} />
    </QueryClientProvider>
  );
}
