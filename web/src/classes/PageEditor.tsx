import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  createClassPage,
  updateClassPage,
  type ClassPage,
  type ClassPageInput,
} from "../api/classes";
import { ApiError } from "../api/client";
import { SelectField } from "../components/SelectField";
import { TextArea, TextField } from "../components/ui/TextField";
import { Button } from "../components/ui/Button";
import { classErrorText } from "./classErrors";
import { tabsToSpaces } from "./ordering";

export const PAGE_KINDS: ClassPageInput["kind"][] = [
  "intro",
  "teachers",
  "events",
  "superlatives",
  "custom",
];
export const BODY_MAX = 4000;
const TITLE_MAX = 120;

/** Create (`page` null) or edit a class page: title and a plain-text body. Any other rule the server enforces shows the server's own message. */
export function PageEditor({
  classId,
  page,
  onDone,
  onCancel,
}: {
  classId: string;
  page: ClassPage | null;
  onDone: (saved: ClassPage) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<ClassPageInput["kind"]>(page?.kind ?? "intro");
  const [placement, setPlacement] = useState<ClassPageInput["placement"]>(
    page?.placement ?? "front",
  );
  const [title, setTitle] = useState(page?.title ?? "");
  const [body, setBody] = useState(page?.body ?? "");
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  const m = useMutation({
    mutationFn: (input: ClassPageInput) =>
      page ? updateClassPage(classId, page.id, input) : createClassPage(classId, input),
    onSuccess: onDone,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    m.mutate({ kind, placement, title: tabsToSpaces(title).trim(), body: tabsToSpaces(body) });
  };
  let msg: string | undefined;
  if (m.error) {
    msg = classErrorText(t, m.error, "assemble");
    // a rule without a message of ours: show what the server said
    if (m.error instanceof ApiError && m.error.status > 0 && msg === t("errors.generic"))
      msg = m.error.message || msg;
  }
  return (
    <form onSubmit={submit} noValidate className="card page-editor" aria-labelledby="page-editor">
      <h2 id="page-editor" ref={heading} tabIndex={-1}>
        {t(page ? "assemble.editor.edit" : "assemble.editor.add")}
      </h2>
      <SelectField
        label={t("assemble.editor.kind")}
        value={kind}
        onChange={(v) => setKind(PAGE_KINDS.find((k) => k === v) ?? "custom")}
        options={PAGE_KINDS.map((k) => ({ value: k, label: t(`assemble.kind.${k}`) }))}
      />
      <SelectField
        label={t("assemble.editor.placement")}
        value={placement}
        onChange={(v) => setPlacement(v === "back" ? "back" : "front")}
        options={(["front", "back"] as const).map((p) => ({
          value: p,
          label: t(`assemble.placement.${p}`),
        }))}
      />
      <TextField
        label={t("assemble.editor.title")}
        value={title}
        onChange={(e) => setTitle(tabsToSpaces(e.target.value))}
        maxLength={TITLE_MAX}
        counter
        autoComplete="off"
      />
      <TextArea
        label={t("assemble.editor.body")}
        value={body}
        onChange={(e) => setBody(tabsToSpaces(e.target.value))}
        maxLength={BODY_MAX}
        counter
        rows={8}
        autoComplete="off"
        help={t("assemble.editor.bodyHelp")}
      />
      {msg && (
        <p role="alert" className="error">
          {msg}
        </p>
      )}
      <div className="row-actions">
        <Button type="submit" variant="primary" busy={m.isPending}>
          {m.isPending ? t("auth.working") : t("assemble.editor.save")}
        </Button>
        <Button onClick={onCancel}>{t("classes.cancel")}</Button>
      </div>
    </form>
  );
}
