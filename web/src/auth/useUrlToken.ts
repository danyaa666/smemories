import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

let stash: { path: string; token: string } | null = null;

/**
 * Called in main.tsx before React renders: moves ?token= out of the address bar (history.replaceState)
 * into memory, so no request (the GET /v1/me of the nav included) can carry it in a Referer.
 */
export function captureUrlToken(): void {
  const { pathname, search } = window.location;
  const token = new URLSearchParams(search).get("token");
  if (token === null) return;
  stash = { path: pathname, token };
  window.history.replaceState(window.history.state, "", pathname);
}

/** The token the page was opened with; whatever is left of the query is cleared in an effect. */
export function useUrlToken(): string {
  const { pathname, search } = useLocation();
  const navigate = useNavigate();
  const [token] = useState(
    () => (stash?.path === pathname ? stash.token : new URLSearchParams(search).get("token")) ?? "",
  );
  useEffect(() => {
    stash = null; // single use: a later visit without a token must not find this one
    if (search) navigate({ search: "" }, { replace: true });
  }, [search, navigate]);
  return token;
}
