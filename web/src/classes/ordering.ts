import type { ClassBook } from "../api/classes";

/** The book order as three lists of ids: the students' pages and the class pages before and after them. */
export type BookOrder = { members: string[]; front: string[]; back: string[] };

export const arrayMove = <T>(a: T[], from: number, to: number): T[] => {
  const b = [...a];
  const [x] = b.splice(from, 1);
  b.splice(to, 0, x as T);
  return b;
};

export const orderOf = (book: ClassBook): BookOrder => ({
  members: book.members.map((m) => m.member_id),
  front: book.front_pages.map((p) => p.id),
  back: book.back_pages.map((p) => p.id),
});

/** `local` keeps the admin's arrangement; ids that are gone drop out and new ids (a page added or moved to this side) go last. */
function mergeList(local: string[], server: string[]): string[] {
  const have = new Set(server);
  const kept = local.filter((id) => have.has(id));
  const seen = new Set(kept);
  return [...kept, ...server.filter((id) => !seen.has(id))];
}
export function reconcile(local: BookOrder | null, server: BookOrder): BookOrder {
  if (!local) return server;
  return {
    members: mergeList(local.members, server.members),
    front: mergeList(local.front, server.front),
    back: mergeList(local.back, server.back),
  };
}

export const sameOrder = (a: BookOrder, b: BookOrder) =>
  (["members", "front", "back"] as const).every(
    (k) => a[k].length === b[k].length && a[k].every((id, i) => id === b[k][i]),
  );

/** The body of class-book/update-order: every list in full, as the API demands. */
export const orderPayload = (classId: string, o: BookOrder) => ({
  class_id: classId,
  member_ids: o.members,
  front_page_ids: o.front,
  back_page_ids: o.back,
});

/** The API refuses a tab in a text field (shared text rule), so a typed or pasted tab becomes two spaces. */
export const tabsToSpaces = (s: string) => s.replace(/\t/g, "  ");
