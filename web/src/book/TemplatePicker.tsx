import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { BookData } from "../api/book";
import { retryOnce } from "../api/notes";
import { listTemplates, type TemplateInfo } from "../api/templates";
import { updateYearbook } from "../api/yearbooks";
import { errorText } from "../auth/errors";
import { Banner } from "../components/ui/Banner";
import { knowsTemplate } from "./templates";

const PLAIN = ""; // template_id null: the built-in layout

/**
 * Chooses the template of the book among the ones the server offers for its paper size and this build can draw (the picker
 * lists what the registry has, so one template or ten both work). The choice is saved with yearbook/update and the cached book
 * is patched with the answer, so the pages redraw without fetching the notes and photos again. The plain layout is always there:
 * it is what a book without a template gets, and picking it clears `template_id`.
 */
export function TemplatePicker({
  bookId,
  size,
  current,
  ask,
}: {
  bookId: string;
  size: string;
  current: string | null;
  /** When given, a pick goes through it: it may ask first and call `go` to carry the pick out, or drop it. */
  ask?: (go: () => void) => void;
}) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const list = useQuery({
    queryKey: ["templates", size],
    queryFn: () => listTemplates(size),
    retry: retryOnce,
    refetchOnWindowFocus: false,
    staleTime: Infinity, // the registry is built into the server: it changes with a deploy
  });
  const [saved, setSaved] = useState("");
  const save = useMutation({
    mutationFn: (id: string) => updateYearbook(bookId, { template_id: id === PLAIN ? null : id }),
    onSuccess: (y) => {
      qc.setQueryData<BookData>(
        ["book-data", bookId],
        (d) => d && { ...d, yearbook: { ...d.yearbook, template_id: y.template_id } },
      );
    },
  });
  const lang = i18n.language.startsWith("vi") ? "vi" : "en";
  const options: TemplateInfo[] = (list.data?.items ?? []).filter(
    (i) => i.renderer === "html" && knowsTemplate(i.id),
  );
  const stored = options.some((o) => o.id === current) ? (current ?? PLAIN) : PLAIN; // an unknown id draws the plain layout
  const chosen = save.isPending ? save.variables : stored; // the radio follows the click at once

  if (list.isPending) return null; // the print screen is usable without it
  if (list.isError) return <Banner variant="warning">{t("print.template.loadError")}</Banner>;
  if (options.length === 0) return null; // nothing to choose from: keep the screen short

  const pick = (id: string, name: string) => {
    if (save.isPending) return; // one save at a time; the fieldset is not disabled, so the focus stays on the radio
    const go = () => {
      setSaved("");
      save.mutate(id, { onSuccess: () => setSaved(t("print.template.saved", { name })) });
    };
    if (ask) ask(go);
    else go();
  };
  return (
    <fieldset className="bk-templates" aria-busy={save.isPending}>
      <legend>{t("print.template.title")}</legend>
      <div className="bk-template-list">
        <Option
          id={PLAIN}
          name={t("print.template.plain")}
          help={t("print.template.plainHelp")}
          checked={chosen === PLAIN}
          onPick={() => pick(PLAIN, t("print.template.plain"))}
        />
        {options.map((o) => (
          <Option
            key={o.id}
            id={o.id}
            name={o.name[lang]}
            preview={o.preview}
            checked={chosen === o.id}
            onPick={() => pick(o.id, o.name[lang])}
          />
        ))}
      </div>
      <p role="status" aria-live="polite">
        {save.isPending ? t("print.template.saving") : saved}
      </p>
      {save.isError && <Banner variant="error">{errorText(t, save.error)}</Banner>}
    </fieldset>
  );
}

function Option({
  id,
  name,
  help,
  preview,
  checked,
  onPick,
}: {
  id: string;
  name: string;
  help?: string;
  preview?: string;
  checked: boolean;
  onPick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <label className={`bk-template${checked ? " bk-template-on" : ""}`}>
      <input type="radio" name="template" value={id} checked={checked} onChange={onPick} />
      {preview && (
        <img
          src={preview}
          alt={t("print.template.preview", { name })}
          width={72}
          height={102}
          loading="lazy"
          onError={(e) => (e.currentTarget.hidden = true)} // a template without a preview image is still choosable
        />
      )}
      <span className="bk-template-name">{name}</span>
      {help && <span className="bk-template-help">{help}</span>}
    </label>
  );
}
