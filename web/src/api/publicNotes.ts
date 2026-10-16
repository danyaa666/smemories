import { ApiError, apiErrorFrom, request, unwrapSuccess } from "./client";

// The public note form (no account, no cookie): docs/api-contract.md section 8, docs/note-fields.md.
// The link token is a bearer secret; it goes in the query string only, as the contract says.

export type NoteField = {
  id: string;
  kind: "short_text" | "long_text";
  label: { en: string; vi: string };
  hint: { en: string; vi: string };
  required: boolean;
  max_length: number;
};

export type NoteForm = {
  yearbook: { title: string };
  owner: { display_name: string };
  deadline_at: number | null;
  open: boolean;
  fields: NoteField[];
};

export const MAX_PHOTOS = 3;
export const MAX_PHOTO_BYTES = 10 * 1024 * 1024;
export const PHOTO_TYPES = ["image/jpeg", "image/png", "image/webp"];

export const getNoteForm = (token: string) =>
  request<NoteForm>(`/public-collect/get?token=${encodeURIComponent(token)}`);

/**
 * POST the note as one multipart request: `answers` first (the server refuses it after the photos), then the optional
 * honeypot, then the photos. XHR because fetch has no upload progress; `onProgress` gets 0..1 for the whole request.
 * A network error is an ApiError with status 0: the answer to a request the server refused early can be lost.
 */
export function submitNote(
  token: string,
  answers: Record<string, string>,
  photos: File[],
  onProgress: (fraction: number) => void,
  honeypot = "",
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/public-collect/submit-note?token=${encodeURIComponent(token)}`);
    xhr.setRequestHeader("Accept", "application/json");
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onerror = () => reject(new ApiError(0, "network_error", "Could not reach the API"));
    xhr.ontimeout = xhr.onerror;
    xhr.onload = () => {
      let body: unknown;
      try {
        body = JSON.parse(xhr.responseText);
      } catch {
        body = undefined;
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        unwrapSuccess(body);
        resolve();
      } else reject(apiErrorFrom(xhr.status, body, (n) => xhr.getResponseHeader(n)));
    };
    const form = new FormData();
    form.append("answers", JSON.stringify(answers));
    if (honeypot) form.append("website", honeypot);
    for (const p of photos) form.append("photos", p);
    xhr.send(form);
  });
}
