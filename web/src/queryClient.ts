import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api/client";
import { isDenied } from "./auth/errors";
import { ME_KEY } from "./auth/useMe";

// networkMode "always": offline, a request fails at once (network_error) instead of pausing.
export const makeQueryClient = (retry: number | boolean = false) => {
  // Any signed-in call can answer ERROR_TERMS_REQUIRED (403) after a version bump: re-read get-me, whose terms.required sends RequireAuth to /accept-terms.
  const onError = (e: unknown) => {
    if (e instanceof ApiError && e.status === 403 && e.code === "terms_required") {
      void qc.invalidateQueries({ queryKey: ME_KEY });
    }
  };
  const qc: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError }),
    mutationCache: new MutationCache({ onError }),
    defaultOptions: {
      queries: { networkMode: "always", retry },
      mutations: { networkMode: "always" },
    },
  });
  // Every screen of /yearbooks/:id shares this query (the shell, Customise, edit): a missing or foreign book must show "not found" at once, not after 3 retries (7 s).
  qc.setQueryDefaults(["yearbook"], {
    retry: typeof retry === "number" ? (n: number, e: unknown) => !isDenied(e) && n < retry : retry,
  });
  return qc;
};
