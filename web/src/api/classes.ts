import type { components } from "./schema";
import { postJson, request } from "./client";

type S = components["schemas"];
export type SchoolClass = S["Class"];
export type ClassInput = S["ClassInput"];
export type ClassMember = S["ClassMember"];
export type ClassInvite = S["ClassInvite"];
export type ClassInviteCreated = S["ClassInviteCreated"];
export type ClassInvitePreview = S["ClassInvitePreview"];
export type ClassRole = SchoolClass["my_role"];
export type ClassPage = S["ClassPage"];
export type ClassBook = S["ClassBookResponse"]["data"];
export type ClassPageInput = Omit<S["ClassPageCreateRequest"], "class_id">;

const enc = encodeURIComponent;
const more = (next?: string) => (next ? `&next_id=${enc(next)}` : "");

export const listClasses = (next?: string) =>
  request<S["ClassListResponse"]["data"]>(`/class/get-list${next ? `?next_id=${enc(next)}` : ""}`);
export const getClass = (id: string) => request<SchoolClass>(`/class/get?id=${enc(id)}`);
export const createClass = (body: ClassInput) => postJson<SchoolClass>("/class/create", body);

export const leaveClass = (classId: string) =>
  postJson<void>("/class-member/leave", { class_id: classId });
/** Link one of the caller's own yearbooks as their page, or `null` to clear it. */
export const setBook = (classId: string, yearbookId: string | null) =>
  postJson<S["ClassPageStatus"]>("/class-member/set-book", {
    class_id: classId,
    yearbook_id: yearbookId,
  });

// Admin only: a student never calls these (the screens mount them for admins only).
export const listMembers = (classId: string, next?: string) =>
  request<S["ClassMemberListResponse"]["data"]>(
    `/class-member/get-list?class_id=${enc(classId)}${more(next)}`,
  );
export const updateRole = (classId: string, memberId: string, role: ClassRole) =>
  postJson<void>("/class-member/update-role", { class_id: classId, member_id: memberId, role });
export const removeMember = (classId: string, memberId: string) =>
  postJson<void>("/class-member/remove", { class_id: classId, member_id: memberId });

export const listInvites = (classId: string, next?: string) =>
  request<S["ClassInviteListResponse"]["data"]>(
    `/class-invite/get-list?class_id=${enc(classId)}${more(next)}`,
  );
export const createInvite = (classId: string, expiresAt: number) =>
  postJson<ClassInviteCreated>("/class-invite/create", {
    class_id: classId,
    expires_at: expiresAt,
  });
export const revokeInvite = (classId: string, id: string) =>
  postJson<void>("/class-invite/revoke", { class_id: classId, id });

// The token travels in the POST body only, never in a URL (docs/class-permissions.md, Invites).
export const lookupInvite = (token: string) =>
  postJson<ClassInvitePreview>("/class-invite/lookup", { token });
export const acceptInvite = (token: string) =>
  postJson<SchoolClass>("/class-invite/accept", { token });

// Admin only: the class book screen (T-129). The book is assembled on read; the order is always the full lists.
export const getClassBook = (classId: string) =>
  request<ClassBook>(`/class-book/get?class_id=${enc(classId)}`);
export const updateOrder = (body: S["ClassBookOrderRequest"]) =>
  postJson<void>("/class-book/update-order", body);
export const createClassPage = (classId: string, page: ClassPageInput) =>
  postJson<ClassPage>("/class-page/create", { class_id: classId, ...page });
export const updateClassPage = (classId: string, id: string, page: ClassPageInput) =>
  postJson<ClassPage>("/class-page/update", { class_id: classId, id, ...page });
export const deleteClassPage = (classId: string, id: string) =>
  postJson<void>("/class-page/delete", { class_id: classId, id });
