import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderResult } from "@testing-library/react";
import { Router } from "wouter";
import { memoryLocation } from "wouter/memory-location";

/**
 * Renders a component with the providers the app supplies at runtime: a
 * React Query client and a router. Retries are off so a failing request
 * surfaces as an error state immediately instead of after a backoff.
 */
export function renderWithProviders(
  ui: ReactElement,
  opts: { route?: string } = {},
): RenderResult & { navigate: (to: string) => void; location: () => string } {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  const { hook, navigate, history } = memoryLocation({ path: opts.route ?? "/", record: true });

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <Router hook={hook}>{children}</Router>
    </QueryClientProvider>
  );

  const result = render(ui, { wrapper });
  return {
    ...result,
    navigate,
    location: () => history[history.length - 1],
  };
}
