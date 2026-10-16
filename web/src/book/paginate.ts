/** Splits `items` into pages of `perPage`; no items, no pages. */
export function paginate<T>(items: readonly T[], perPage: number): T[][] {
  const n = Math.max(1, Math.floor(perPage));
  const pages: T[][] = [];
  for (let i = 0; i < items.length; i += n) pages.push(items.slice(i, i + n));
  return pages;
}
