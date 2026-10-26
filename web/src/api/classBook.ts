import type { components } from "./schema";
import { request } from "./client";

// `GET /api/class-book/get` (api/openapi.yaml): the class book for the browser renderer. Class admin only (a student gets 403, a non-member 404).

export type ClassBook = components["schemas"]["ClassBookResponse"]["data"];
export type ClassBookMember = ClassBook["members"][number];
export type ClassBookPage = ClassBook["front_pages"][number];

/** Up to 60 members of 40 notes: about 1.2 MB, read as one plain fetch. Unknown fields of the answer are ignored. */
export const getClassBook = (classId: string) =>
  request<ClassBook>(`/class-book/get?class_id=${encodeURIComponent(classId)}`);
