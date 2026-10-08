import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

/**
 * Reads ?token= once and removes the query from the address bar (history.replaceState) in an
 * effect that runs before any effect declared later in the page, so the token is never kept in the
 * URL, the history or a Referer.
 */
export function useUrlToken(): string {
  const { search } = useLocation();
  const navigate = useNavigate();
  const [token] = useState(() => new URLSearchParams(search).get("token") ?? "");
  useEffect(() => {
    if (search) navigate({ search: "" }, { replace: true });
  }, [search, navigate]);
  return token;
}
