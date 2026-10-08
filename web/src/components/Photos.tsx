import { useMutation } from "@tanstack/react-query";
import { useRef, useState, type ChangeEvent } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api/client";
import {
  deleteMedia,
  getYearbook,
  mediaUrl,
  profileInput,
  replaceProfile,
  updateYearbook,
  uploadMedia,
  type Media,
  type Yearbook,
} from "../api/yearbooks";
import { errorText } from "../auth/errors";

export const BUSY_RETRIES = 3; // 503 busy: wait Retry-After (2 s if absent) and try again, this many times

type Upload = { key: number; name: string; progress: number; busy?: boolean; error?: unknown };

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function uploadWithRetry(
  id: string,
  file: File,
  onProgress: (f: number) => void,
  onBusy: () => void,
): Promise<Media> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await uploadMedia(id, file, onProgress);
    } catch (e) {
      if (!(e instanceof ApiError && e.code === "busy") || attempt >= BUSY_RETRIES) throw e;
      onBusy();
      await sleep((e.retryAfter ?? 2) * 1000);
    }
  }
}

/**
 * Upload, thumbnails, delete, cover and profile photo.
 * ponytail: the API cannot list a book's photos yet, so the grid holds this session's uploads plus the
 * cover and profile photo the book already points at; swap `session` for a list query when that endpoint exists.
 */
export function Photos({
  yearbook,
  onChanged,
}: {
  yearbook: Yearbook;
  onChanged: (y: Yearbook) => void;
}) {
  const { t } = useTranslation();
  const [session, setSession] = useState<string[]>([]);
  const [uploads, setUploads] = useState<Upload[]>([]);
  const nextKey = useRef(0);
  const cover = yearbook.cover_media_id;
  const profilePhoto = yearbook.profile.photo_media_id;
  const ids = [...new Set([...session, cover, profilePhoto].filter((x): x is string => !!x))];

  const act = useMutation({
    mutationFn: (job: () => Promise<Yearbook>) => job(),
    onSuccess: onChanged,
  });

  const patch = (key: number, p: Partial<Upload>) =>
    setUploads((u) => u.map((x) => (x.key === key ? { ...x, ...p } : x)));

  async function onFiles(e: ChangeEvent<HTMLInputElement>) {
    const files = [...(e.target.files ?? [])];
    e.target.value = ""; // the same file can be picked again
    const items = files.map((f) => ({ key: nextKey.current++, name: f.name, progress: 0 }));
    setUploads((u) => [...u, ...items]);
    let blocked: unknown; // a limit that makes the rest of the batch pointless
    for (const [i, file] of files.entries()) {
      const key = items[i]!.key;
      if (blocked) {
        patch(key, { error: blocked });
        continue;
      }
      try {
        const m = await uploadWithRetry(
          yearbook.id,
          file,
          (progress) => patch(key, { progress, busy: false }),
          () => patch(key, { busy: true }),
        );
        setSession((s) => [...s, m.id]);
        setUploads((u) => u.filter((x) => x.key !== key));
      } catch (err) {
        patch(key, { error: err, busy: false });
        if (err instanceof ApiError && (err.status === 429 || err.code === "quota_exceeded")) {
          blocked = err;
        }
      }
    }
  }

  const toggleCover = (id: string) =>
    act.mutate(() => updateYearbook(yearbook.id, { cover_media_id: cover === id ? null : id }));
  const toggleProfile = (id: string) =>
    act.mutate(() =>
      replaceProfile(yearbook.id, {
        ...profileInput(yearbook.profile),
        photo_media_id: profilePhoto === id ? null : id,
      }),
    );
  const remove = (id: string) => {
    if (!window.confirm(t("photos.confirmDelete"))) return;
    act.mutate(async () => {
      await deleteMedia(id);
      setSession((s) => s.filter((x) => x !== id));
      return getYearbook(yearbook.id); // the server clears a cover or profile photo that pointed at it
    });
  };

  return (
    <section aria-labelledby="photos-h">
      <h2 id="photos-h">{t("photos.title")}</h2>
      <p>
        <label className="button">
          {t("photos.add")}
          <input
            type="file"
            multiple
            accept="image/jpeg,image/png,image/webp"
            className="visually-hidden"
            onChange={(e) => void onFiles(e)}
          />
        </label>
      </p>
      <p>
        <small>{t("photos.hint")}</small>
      </p>
      {uploads.length > 0 && (
        <ul className="uploads">
          {uploads.map((u) => (
            <li key={u.key}>
              <span>{u.name}</span>{" "}
              {u.error ? (
                <span role="alert" className="error">
                  {errorText(t, u.error)}
                </span>
              ) : u.busy ? (
                <span role="status">{t("photos.busy")}</span>
              ) : (
                <progress value={u.progress} max={1} aria-label={u.name} />
              )}
              {u.error !== undefined && (
                <button
                  type="button"
                  onClick={() => setUploads((x) => x.filter((y) => y.key !== u.key))}
                >
                  {t("photos.dismiss")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {act.isError && (
        <p role="alert" className="error">
          {errorText(t, act.error)}
        </p>
      )}
      {ids.length === 0 ? (
        <p>{t("photos.empty")}</p>
      ) : (
        <ul className="photo-grid">
          {ids.map((id, n) => (
            <li key={id}>
              <img src={mediaUrl(id)} alt={t("photos.alt", { n: n + 1 })} loading="lazy" />
              <div className="photo-actions">
                <button
                  type="button"
                  aria-pressed={cover === id}
                  disabled={act.isPending}
                  onClick={() => toggleCover(id)}
                >
                  {t("photos.cover")}
                </button>
                <button
                  type="button"
                  aria-pressed={profilePhoto === id}
                  disabled={act.isPending}
                  onClick={() => toggleProfile(id)}
                >
                  {t("photos.profile")}
                </button>
                <button type="button" disabled={act.isPending} onClick={() => remove(id)}>
                  {t("photos.delete")}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
