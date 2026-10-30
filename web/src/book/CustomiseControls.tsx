import type { TFunction } from "i18next";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { DesignBlock } from "../api/bookDesign";
import { Button } from "../components/ui/Button";
import { TextField } from "../components/ui/TextField";
import { moveSection, sectionOf, type Draft } from "./customise";
import type { Template } from "./templates";

// The controls of the customise screen (epic E09, T-144). The names of themes, font sets, sections, texts and pictures come from the template's own
// locale keys (`customise.tpl.<template id>.*`), not from the server; an id without a name shows the id. Every control edits the draft and tells the
// screen which part of the book it concerns (`onFocus`), so the preview shows those pages.

const tplKey = (tpl: string, rest: string) => `customise.tpl.${tpl}.${rest}`;
const idKey = (id: string) => id.replaceAll(".", "_");

type Props = {
  tpl: Template;
  /** The template id of the book (the locale keys), which may be one this build has no page components for. */
  tplId: string;
  block: DesignBlock;
  draft: Draft;
  edit: (change: (d: Draft) => Draft) => void;
  /** The book's own language for the placeholders (the template's wording of each text). */
  bookT: TFunction;
  /** Server messages by text id. */
  textErrors: Record<string, string>;
  announce: (message: string) => void;
  onFocus: (section: string | null) => void;
};

export function CustomiseControls(p: Props) {
  const { t } = useTranslation();
  const name = (rest: string, id: string) => t(tplKey(p.tplId, rest), { defaultValue: id });
  return (
    <div className="cu-controls">
      <Group legend={t("customise.colours")} onFocus={() => p.onFocus(null)}>
        <Choices
          name="theme"
          value={p.draft.theme}
          onPick={(theme) => p.edit((d) => ({ ...d, theme }))}
          items={p.block.themes.map((id) => ({
            id,
            label: name(`themes.${id}`, id),
            extra:
              // the colours of the theme are the template's own custom properties (memory.css, navy.css), read through the theme attribute
              p.tpl.swatch && (
                <span
                  className={`${p.tpl.swatch.root} cu-chips`}
                  data-theme={id}
                  aria-hidden="true"
                >
                  {p.tpl.swatch.chips.map((c) => (
                    <i key={c} style={{ background: `var(${c})` }} />
                  ))}
                </span>
              ),
          }))}
        />
      </Group>

      <Group legend={t("customise.fonts")} onFocus={() => p.onFocus(null)}>
        <Choices
          name="fonts"
          value={p.draft.fonts}
          onPick={(fonts) => p.edit((d) => ({ ...d, fonts }))}
          items={p.block.font_sets.map((id) => ({
            id,
            label: name(`fonts.${id}`, id),
            extra: p.tpl.swatch && (
              <span className={`${p.tpl.swatch.root} cu-sample`} data-fonts={id} aria-hidden="true">
                <span className={p.tpl.swatch.serif}>{name("fontSample", "Aa")}</span>
              </span>
            ),
          }))}
        />
      </Group>

      <Sections {...p} />

      <Group legend={t("customise.texts")} help={t("customise.textsHelp")}>
        {p.block.texts.map((x) => {
          const key = p.tpl.textDefaults?.[x.id];
          return (
            <TextField
              key={x.id}
              label={name(`texts.${idKey(x.id)}`, x.id)}
              value={p.draft.texts[x.id] ?? ""}
              maxLength={x.max}
              counter
              placeholder={key ? p.bookT(key).replace(/\s*\n\s*/g, " ") : ""}
              error={p.textErrors[x.id]}
              onFocus={() => p.onFocus(sectionOf(x.id))}
              onChange={(e) => {
                const v = e.target.value;
                p.edit((d) => ({ ...d, texts: { ...d.texts, [x.id]: v } }));
              }}
            />
          );
        })}
      </Group>

      {p.block.images.map((im) => (
        <Group
          key={im.id}
          legend={name(`images.${idKey(im.id)}.label`, im.id)}
          onFocus={() => p.onFocus(sectionOf(im.id))}
        >
          <Choices
            name={`image-${im.id}`}
            value={p.draft.images[im.id] ?? ""}
            onPick={(c) => p.edit((d) => ({ ...d, images: { ...d.images, [im.id]: c } }))}
            items={im.choices.map((c) => ({
              id: c,
              label: name(`images.${idKey(im.id)}.choices.${c}`, c),
            }))}
          />
        </Group>
      ))}
    </div>
  );
}

function Group({
  legend,
  help,
  onFocus,
  children,
}: {
  legend: string;
  help?: string;
  onFocus?: () => void;
  children: ReactNode;
}) {
  return (
    // focus events bubble up from the controls inside; the fieldset itself is not a control
    <fieldset className="cu-group" onFocus={onFocus}>
      <legend>{legend}</legend>
      {help && <p className="cu-help">{help}</p>}
      {children}
    </fieldset>
  );
}

/** One choice among several (radio buttons): the label, an optional sample, and the word "Selected" on the chosen one, so the state is not shown by colour alone. */
function Choices({
  name,
  value,
  items,
  onPick,
}: {
  name: string;
  value: string;
  items: { id: string; label: string; extra?: ReactNode }[];
  onPick: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="cu-choices">
      {items.map((it) => (
        <label key={it.id} className={`cu-choice${it.id === value ? " cu-choice-on" : ""}`}>
          <input
            type="radio"
            name={name}
            value={it.id}
            checked={it.id === value}
            onChange={() => onPick(it.id)}
          />
          <span className="cu-choice-name">{it.label}</span>
          {it.extra}
          {it.id === value && <span className="cu-selected">{t("customise.selected")}</span>}
        </label>
      ))}
    </div>
  );
}

/** The sections: a switch to leave a hideable one out, move up and down for a movable one (no dragging), "Fixed" for the rest. */
function Sections({ block, draft, edit, tplId, announce, onFocus }: Props) {
  const { t } = useTranslation();
  const list = useRef<HTMLOListElement>(null);
  const [refocus, setRefocus] = useState<{ id: string; ctl: "up" | "down" } | null>(null); // a new object per move
  const label = (id: string) => t(tplKey(tplId, `sections.${id}`), { defaultValue: id });

  useEffect(() => {
    if (!refocus) return;
    // the node of a moved row is re-inserted and may lose focus: put it back on the control that was used, or on the other one when it just reached an end
    const row = list.current?.querySelector<HTMLElement>(`[data-sec="${refocus.id}"]`);
    for (const c of [refocus.ctl, refocus.ctl === "up" ? "down" : "up"]) {
      const b = row?.querySelector<HTMLButtonElement>(`[data-ctl="${c}"]`);
      if (b && !b.disabled) return b.focus();
    }
  }, [refocus]);

  const move = (id: string, step: -1 | 1) => {
    const next = moveSection(block, draft.order, id, step);
    if (!next) return;
    setRefocus({ id, ctl: step < 0 ? "up" : "down" });
    edit((d) => ({ ...d, order: next }));
    announce(
      t("customise.moved", { name: label(id), position: next.indexOf(id) + 1, total: next.length }),
    );
  };
  const toggle = (id: string, shown: boolean) => {
    edit((d) => ({
      ...d,
      hidden: shown ? d.hidden.filter((h) => h !== id) : [...d.hidden.filter((h) => h !== id), id],
    }));
    announce(t(shown ? "customise.nowShown" : "customise.nowHidden", { name: label(id) }));
  };

  return (
    <Group
      legend={t("customise.sections")}
      help={t("customise.sectionsHelp")}
      onFocus={() => onFocus(null)}
    >
      <ol ref={list} className="cu-sections">
        {draft.order.map((id, i) => {
          const s = block.sections.find((x) => x.id === id);
          if (!s) return null;
          const shown = !draft.hidden.includes(id);
          const can = (step: -1 | 1) => moveSection(block, draft.order, id, step) !== null;
          return (
            <li key={id} data-sec={id} className="cu-section">
              <span className="cu-pos">{i + 1}</span>
              <span className="cu-section-name">{label(id)}</span>
              {s.hideable && (
                <label className="cu-switch">
                  <input
                    type="checkbox"
                    role="switch"
                    checked={shown}
                    aria-label={t("customise.showSection", { name: label(id) })}
                    onChange={(e) => toggle(id, e.target.checked)}
                  />
                  <span>{shown ? t("customise.shown") : t("customise.hidden")}</span>
                </label>
              )}
              {s.movable ? (
                <span className="cu-moves">
                  <Button
                    variant="ghost"
                    data-ctl="up"
                    disabled={!can(-1)}
                    aria-label={t("customise.moveUp", { name: label(id) })}
                    onClick={() => move(id, -1)}
                  >
                    <span aria-hidden="true">↑</span>
                  </Button>
                  <Button
                    variant="ghost"
                    data-ctl="down"
                    disabled={!can(1)}
                    aria-label={t("customise.moveDown", { name: label(id) })}
                    onClick={() => move(id, 1)}
                  >
                    <span aria-hidden="true">↓</span>
                  </Button>
                </span>
              ) : (
                <span className="cu-locked">
                  {s.fixed === "first"
                    ? t("customise.lockedFirst")
                    : s.fixed === "last"
                      ? t("customise.lockedLast")
                      : t("customise.locked")}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </Group>
  );
}
