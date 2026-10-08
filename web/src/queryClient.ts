import { QueryClient } from "@tanstack/react-query";

// networkMode "always": offline, a request fails at once (network_error) instead of pausing.
export const makeQueryClient = (retry: number | boolean = false) =>
  new QueryClient({
    defaultOptions: {
      queries: { networkMode: "always", retry },
      mutations: { networkMode: "always" },
    },
  });
