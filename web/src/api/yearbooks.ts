import type { components } from "./schema";
import { ApiError, apiErrorFrom, postJson, request, sendJson } from "./client";

type S = components["schemas"];
export type Yearbook = S["Yearbook"];
export type Profile = S["Profile"];
export type Media = S["Media"];
export type YearbookCreate = S["YearbookCreate"];
export type YearbookPatch = S["YearbookPatch"];
export type ProfileInput = S["ProfileInput"];

const book = (id: string) => `/v1/yearbooks/${encodeURIComponent(id)}`;

export const listYearbooks = (cursor?: string) =>
  request<{ yearbooks: Yearbook[]; next_cursor: string | null }>(
    `/v1/yearbooks${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`,
  );

export const getYearbook = async (id: string) =>
  (await request<S["YearbookEnvelope"]>(book(id))).yearbook;

export const createYearbook = async (body: YearbookCreate) =>
  (await postJson<S["YearbookEnvelope"]>("/v1/yearbooks", body)).yearbook;

export const updateYearbook = async (id: string, body: YearbookPatch) =>
  (await sendJson<S["YearbookEnvelope"]>("PATCH", book(id), body)).yearbook;

/** PUT replaces the whole profile: whatever is not in `body` (the photo included) is cleared. */
export const replaceProfile = async (id: string, body: ProfileInput) =>
  (await sendJson<S["YearbookEnvelope"]>("PUT", `${book(id)}/profile`, body)).yearbook;

/** The current profile as a PUT body, so a save that changes one field keeps the rest, the photo included. */
export const profileInput = (p: Profile): ProfileInput => ({
  full_name: p.full_name,
  nickname: p.nickname,
  birthday: p.birthday,
  quote: p.quote,
  hobbies: p.hobbies,
  future_plans: p.future_plans,
  photo_media_id: p.photo_media_id,
});

export const deleteYearbook = (id: string) => sendJson<void>("DELETE", book(id));
export const deleteMedia = (id: string) =>
  sendJson<void>("DELETE", `/v1/media/${encodeURIComponent(id)}`);

/** Same-origin image URL; the session cookie authorises it. */
export const mediaUrl = (id: string, size: "thumb" | "display" = "thumb") =>
  `/api/v1/media/${encodeURIComponent(id)}/content?size=${size}`;

const DEFAULT_MAX_BYTES = 10 * 1024 * 1024; // SMEM_MEDIA_MAX_BYTES default

/** fetch has no upload progress, so the upload uses XHR. `onProgress` gets 0..1. */
export function uploadMedia(
  yearbookId: string,
  file: File,
  onProgress: (fraction: number) => void,
): Promise<Media> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/${book(yearbookId).slice(1)}/media`);
    xhr.setRequestHeader("Accept", "application/json");
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onerror = () =>
      // A server that refuses an oversized body early may reset the connection before the browser sees the 413.
      reject(
        file.size > DEFAULT_MAX_BYTES
          ? new ApiError(413, "payload_too_large", "File too large")
          : new ApiError(0, "network_error", "Could not reach the API"),
      );
    xhr.onload = () => {
      let body: unknown;
      try {
        body = JSON.parse(xhr.responseText);
      } catch {
        body = undefined;
      }
      if (xhr.status === 201) resolve((body as S["MediaEnvelope"]).media);
      else reject(apiErrorFrom(xhr.status, body, (n) => xhr.getResponseHeader(n)));
    };
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}
