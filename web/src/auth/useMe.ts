import { useQuery } from "@tanstack/react-query";
import { getMe } from "../api/auth";

export const ME_KEY = ["me"] as const;

/** Session bootstrap: the user from GET /v1/me, or null when signed out. */
export const useMe = () =>
  useQuery({ queryKey: ME_KEY, queryFn: getMe, retry: false, staleTime: Infinity });
