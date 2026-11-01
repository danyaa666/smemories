import { useRef, useState, useSyncExternalStore, type ChangeEvent, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { mediaUrl, type Media } from "../../../api/yearbooks";
import { ACCEPTED, MAX_DESIGN_IMAGES, type UploadFailure, type Uploads } from "./uploads";
import type { DesignImages as Images } from "./useDesignImages";

/** The words for a failed upload (editor.upload.error.<code>), with its numbers. */
export function useFailureText() {
  const { t } = useTranslation();
  return (f: UploadFailure) =>
    t(`editor.upload.error.${f.code}`, {
      count: f.count ?? MAX_DESIGN_IMAGES,
      max: MAX_DESIGN_IMAGES,
      seconds: f.seconds,
      defaultValue: t("editor.upload.error.generic"),
    });
}

/**
 * The student's own images of this book (T-162): an upload button and a drop zone, the uploads on their way (progress, cancel, the reason of a failure), and
 * the thumbnails. Picking one calls `onPick`; `onDelete` adds a delete button to each. Thumbnails are native lazy images, so one that is not near the
 * screen is not fetched.
 */
export function DesignImages({
  images,
  uploads,
  onPick,
  onDelete,
  selectedId,
  pickLabel,
  readOnly,
}: {
  images: Images;
  uploads: Uploads;
  onPick: (m: Media) => void;
  onDelete?: (m: Media) => void;
  /** The image that is in use (a background), marked pressed. */
  selectedId?: string;
  /** The accessible name of a tile, given its position and the total ("Add image 2 of 5"). */
  pickLabel: (n: number, total: number) => string;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const failure = useFailureText();
  const jobs = useSyncExternalStore(uploads.subscribe, uploads.getSnapshot);
  const file = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const shown = jobs.filter((j) => j.state !== "done");
  const send = (list: FileList | null) => {
    if (list && !readOnly) uploads.add([...list]);
  };
  const drop = (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    send(e.dataTransfer.files);
  };
  return (
    <div className="ed-images">
      {/* a drop target; the button inside is the keyboard way to the same thing */}
      <section
        className={`ed-drop${over ? " is-over" : ""}`}
        aria-label={t("editor.upload.zone")}
        onDragOver={(e) => {
          e.preventDefault();
          setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
      >
        <input
          ref={file}
          type="file"
          hidden
          multiple
          accept={ACCEPTED.join(",")}
          data-testid="design-file"
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            send(e.target.files);
            e.target.value = ""; // the same file can be chosen again
          }}
        />
        <button type="button" disabled={readOnly} onClick={() => file.current?.click()}>
          {t("editor.upload.button")}
        </button>
        <span className="ed-hint">{t("editor.upload.hint")}</span>
      </section>
      {shown.length > 0 && (
        <ul className="ed-jobs">
          {shown.map((j) => (
            <li key={j.key}>
              <span className="ed-job-name">{j.name}</span>
              {j.state === "failed" && j.failure ? (
                <>
                  <span role="alert" className="ed-warn">
                    {failure(j.failure)}{" "}
                    {j.failure.code === "notVerified" && (
                      <Link to="/verify-email" target="_blank" rel="noopener">
                        {t("editor.upload.verifyLink")}
                      </Link>
                    )}
                  </span>
                  <button type="button" onClick={() => uploads.dismiss(j.key)}>
                    {t("editor.upload.dismiss")}
                  </button>
                </>
              ) : (
                <>
                  <progress
                    max={1}
                    value={j.state === "queued" ? 0 : j.progress}
                    aria-label={t("editor.upload.progress", { name: j.name })}
                  />
                  <span className="ed-hint">
                    {j.state === "queued"
                      ? t("editor.upload.waiting")
                      : `${Math.round(j.progress * 100)} %`}
                  </span>
                  <button type="button" onClick={() => uploads.cancel(j.key)}>
                    {t("editor.upload.cancel")}
                  </button>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      {images.status === "failed" && (
        <p role="alert" className="ed-warn">
          {t("editor.upload.listFailed")}{" "}
          <button type="button" onClick={images.reload}>
            {t("auth.retry")}
          </button>
        </p>
      )}
      {images.status === "loading" && <p className="ed-hint">{t("editor.upload.loading")}</p>}
      {images.status === "ready" && images.items.length === 0 && (
        <p className="ed-hint">{t("editor.upload.empty")}</p>
      )}
      {images.items.length > 0 && !images.more && (
        <p className="ed-hint">
          {t("editor.upload.count", { count: images.items.length, max: MAX_DESIGN_IMAGES })}
        </p>
      )}
      <ul className="ed-grid-tiles ed-thumbs">
        {images.items.map((m, i) => (
          <li key={m.id}>
            <button
              type="button"
              className="ed-tile"
              aria-label={pickLabel(i + 1, images.items.length)}
              aria-pressed={selectedId === undefined ? undefined : selectedId === m.id}
              disabled={readOnly}
              onClick={() => onPick(m)}
            >
              <img src={mediaUrl(m.id, "thumb")} alt="" loading="lazy" decoding="async" />
            </button>
            {onDelete && (
              <button
                type="button"
                className="ed-tile-del"
                aria-label={t("editor.upload.deleteN", { n: i + 1, total: images.items.length })}
                disabled={readOnly}
                onClick={() => onDelete(m)}
              >
                ×
              </button>
            )}
          </li>
        ))}
      </ul>
      {images.more && (
        <button type="button" disabled={images.loadingMore} onClick={images.loadMore}>
          {t("editor.upload.more")}
        </button>
      )}
    </div>
  );
}
