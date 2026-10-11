/** Old email links carried ?token=; nothing reads it now, so keep it out of history and referrers. */
export function dropTokenParam() {
  const url = new URL(window.location.href);
  if (!url.searchParams.has("token")) return;
  url.searchParams.delete("token");
  window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
}
