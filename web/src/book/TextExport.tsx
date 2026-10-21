import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getBookText, revokePhotoLinks, type BookText } from "../api/bookExport";
import { errorText } from "../auth/errors";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { Toast } from "../components/ui/Toast";

type Job = "download" | "copy" | "revoke";
const TOAST_MS = 4000;

/** Saves `text` as a file: a Blob behind an object URL, clicked through an anchor, revoked right after. */
function saveFile(text: string, filename: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.append(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

/**
 * "Export as text" (D-32): the book as one text file with a public link for every photo. The first download or copy after the
 * page is opened asks the owner to confirm that the links are public; "Disable photo links" ends every link made so far, and the
 * warning comes back with the next export. The section never blocks printing: an error here is a message, nothing else.
 */
export function TextExport({ bookId }: { bookId: string }) {
  const { t } = useTranslation();
  const [warned, setWarned] = useState(false);
  const [dialog, setDialog] = useState<"warn" | "revoke" | null>(null);
  const [want, setWant] = useState<"download" | "copy">("download"); // what the warning dialog is for
  const [busy, setBusy] = useState<Job | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [partial, setPartial] = useState<BookText | null>(null); // the last export, when it left notes out
  const [manual, setManual] = useState(""); // text to copy by hand when the browser refuses
  const [toast, setToast] = useState("");
  const running = useRef(false); // a click right behind a click, before the buttons are disabled
  const timer = useRef<number>(undefined);
  const manualBox = useRef<HTMLTextAreaElement>(null);

  useEffect(() => () => window.clearTimeout(timer.current), []);
  useEffect(() => {
    manualBox.current?.select();
  }, [manual]);

  const say = (message: string) => {
    window.clearTimeout(timer.current);
    setToast(message);
    timer.current = window.setTimeout(() => setToast(""), TOAST_MS);
  };

  async function run(job: Job, work: () => Promise<void>) {
    if (running.current) return;
    running.current = true;
    setBusy(job);
    setError(null);
    try {
      await work();
    } catch (e) {
      setError(e);
    } finally {
      running.current = false;
      setBusy(null);
    }
  }

  async function exportText(job: "download" | "copy") {
    await run(job, async () => {
      const book = await getBookText(bookId);
      setPartial(book.truncated ? book : null);
      setManual("");
      if (job === "download") return saveFile(book.text, book.filename);
      if (await copy(book.text)) say(t("print.export.copied"));
      else setManual(book.text);
    });
  }

  const start = (job: "download" | "copy") => {
    if (warned) return void exportText(job);
    setWant(job);
    setDialog("warn");
  };

  const disabled = busy !== null;
  const label = (job: Job, text: string) => (busy === job ? t("print.export.working") : text);
  return (
    <section className="bk-export" aria-labelledby="bk-export-title">
      <h2 id="bk-export-title">{t("print.export.title")}</h2>
      <p>{t("print.export.help")}</p>
      <div className="bk-export-buttons">
        <Button disabled={disabled} busy={busy === "download"} onClick={() => start("download")}>
          {label("download", t("print.export.download"))}
        </Button>
        <Button disabled={disabled} busy={busy === "copy"} onClick={() => start("copy")}>
          {label("copy", t("print.export.copy"))}
        </Button>
        <Button
          variant="ghost"
          disabled={disabled}
          busy={busy === "revoke"}
          onClick={() => setDialog("revoke")}
        >
          {label("revoke", t("print.export.disable"))}
        </Button>
      </div>
      {partial && (
        <Banner variant="warning">
          {t("print.export.truncated", {
            note_count: partial.note_count,
            approved_total: partial.approved_total,
          })}
        </Banner>
      )}
      {error !== null && <Banner variant="error">{errorText(t, error)}</Banner>}
      {manual && (
        <>
          <p role="status">{t("print.export.copyManual")}</p>
          <textarea
            ref={manualBox}
            className="bk-export-text"
            readOnly
            rows={8}
            value={manual}
            aria-label={t("print.export.textLabel")}
          />
        </>
      )}
      {toast && (
        <div className="inbox-toast">
          <Toast message={toast} />
        </div>
      )}
      {dialog === "warn" && (
        <Dialog
          title={t("print.export.warnTitle")}
          confirmLabel={t("print.export.continue")}
          cancelLabel={t("print.export.cancel")}
          onCancel={() => setDialog(null)}
          onConfirm={() => {
            setWarned(true);
            setDialog(null);
            void exportText(want);
          }}
        >
          <p>{t("print.export.warn")}</p>
        </Dialog>
      )}
      {dialog === "revoke" && (
        <Dialog
          danger
          title={t("print.export.disableTitle")}
          confirmLabel={t("print.export.disableConfirm")}
          cancelLabel={t("print.export.cancel")}
          busy={busy === "revoke"}
          onCancel={() => busy === null && setDialog(null)}
          onConfirm={() =>
            void run("revoke", async () => {
              try {
                await revokePhotoLinks(bookId);
                setWarned(false); // the next export asks again
                setManual(""); // the text on screen holds links that no longer work
                say(t("print.export.disabled"));
              } finally {
                setDialog(null);
              }
            })
          }
        >
          <p>{t("print.export.disableBody")}</p>
        </Dialog>
      )}
    </section>
  );
}

/** The clipboard API, then the old way through a selected textarea. False when the browser refused both. */
async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // no clipboard API, or refused (no permission, or the user gesture is gone after the request): try execCommand
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.readOnly = true;
  ta.setAttribute("aria-hidden", "true");
  ta.style.cssText = "position:fixed;top:0;left:0;opacity:0";
  document.body.append(ta);
  ta.select();
  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    ta.remove();
  }
}
