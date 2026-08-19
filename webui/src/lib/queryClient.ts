import { QueryClient } from "@tanstack/react-query";

// One client per workspace tab in the desktop shell (warm cache per tab),
// a single one in the browser.
export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: 1,
        refetchOnWindowFocus: false,
      },
    },
  });
}
