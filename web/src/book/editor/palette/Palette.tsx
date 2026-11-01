import { useRef, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import type { Media } from "../../../api/yearbooks";
import type { Asset, AssetKind } from "../../assets";
import type { TemplateLayout } from "../../layout/runtime";
import { setBg, type LayoutDoc, type Op, type Refusal } from "../document";
import { Swatches } from "../Swatches";
import { AssetGrid, assetLabel } from "./AssetGrid";
import { assetItem, imageItem } from "./defaults";
import { DesignImages } from "./DesignImages";
import type { Uploads } from "./uploads";
import type { DesignImages as Images } from "./useDesignImages";
import { newItemId, type LayoutItem } from "../document";

export const TABS = ["sticker", "shape", "frame", "images", "background"] as const;
export type Tab = (typeof TABS)[number];
const ASSET_TABS: Partial<Record<Tab, AssetKind>> = {
  sticker: "sticker",
  shape: "shape",
  frame: "frame",
};
/** The page types of a layout (docs/templates.md): the notes type is drawn on every notes page. */
export const ALL_KINDS = ["cover", "profile", "notes", "back"];
const BG_KINDS: readonly AssetKind[] = ["shape", "sticker"];

/**
 * The palette (T-162): a side panel with five tabs. Stickers, Shapes and Frames add an asset item to the edited page; My images adds a design image
 * (and uploads or deletes them); Background sets the page background (a colour, a registry asset or a design image) of the page kind being edited.
 */
export function Palette({
  tab,
  onTab,
  kind,
  kinds,
  doc,
  apply,
  readOnly,
  announce,
  spec,
  scope,
  images,
  uploads,
  onAdd,
  onAskDelete,
}: {
  tab: Tab;
  onTab: (t: Tab) => void;
  /** The page kind being edited, and every page kind of the book (for "apply to all"). */
  kind: string;
  kinds: readonly string[];
  doc: LayoutDoc;
  apply: (op: Op) => Refusal | undefined;
  readOnly: boolean;
  announce: (s: string) => void;
  spec: TemplateLayout | undefined;
  scope: { theme?: string; fonts?: string };
  images: Images;
  uploads: Uploads;
  /** Adds an item to the edited page (the shell applies it, reports a refusal, selects it). */
  onAdd: (item: LayoutItem, what: string) => void;
  onAskDelete: (m: Media) => void;
}) {
  const { t, i18n } = useTranslation();
  const tabs = useRef<HTMLDivElement>(null);
  const bg = doc.pages[kind]?.bg;
  const setBackground = (next: NonNullable<typeof bg> | null, what: string) => {
    if (!apply(setBg(kind, next))) announce(what);
  };
  const applyToAll = () => {
    if (!bg) return;
    const r = apply((d) => kinds.reduce((acc, k) => setBg(k, bg)(acc.doc), { doc: d }));
    if (!r) announce(t("editor.palette.bgAllDone"));
  };
  const key = (e: KeyboardEvent) => {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const next = TABS[(TABS.indexOf(tab) + d + TABS.length) % TABS.length]!;
    onTab(next);
    tabs.current?.querySelector<HTMLElement>(`#ed-tab-${next}`)?.focus();
  };
  const assetKind = ASSET_TABS[tab];
  const pickLabelAdd = (n: number, total: number) => t("editor.palette.addImageN", { n, total });
  return (
    <aside className="ed-panel ed-palette" aria-label={t("editor.palette.title")}>
      <h2>{t("editor.palette.title")}</h2>
      <div ref={tabs} role="tablist" aria-label={t("editor.palette.tabs")} className="ed-tabs">
        {TABS.map((id) => (
          <button
            key={id}
            id={`ed-tab-${id}`}
            type="button"
            role="tab"
            aria-selected={tab === id}
            aria-controls="ed-tabpanel"
            tabIndex={tab === id ? 0 : -1}
            onClick={() => onTab(id)}
            onKeyDown={key}
          >
            {t(`editor.palette.tab.${id}`)}
          </button>
        ))}
      </div>
      <div id="ed-tabpanel" role="tabpanel" aria-labelledby={`ed-tab-${tab}`}>
        {assetKind && (
          <AssetGrid
            key={assetKind}
            kinds={[assetKind]}
            label={t(`editor.palette.tab.${tab}`)}
            disabled={readOnly || !spec}
            onPick={(a: Asset) =>
              onAdd(
                assetItem(a, newItemId()),
                t("editor.palette.added", { name: assetLabel(a, i18n.language) }),
              )
            }
          />
        )}
        {tab === "images" && (
          <DesignImages
            images={images}
            uploads={uploads}
            readOnly={readOnly || !spec}
            pickLabel={pickLabelAdd}
            onPick={(m) => onAdd(imageItem(m, newItemId()), t("editor.palette.addedImage"))}
            onDelete={onAskDelete}
          />
        )}
        {tab === "background" && (
          <div className="ed-bgtab">
            <p className="ed-hint">
              {t("editor.palette.bgNote", { page: t(`editor.palette.kind.${kind}`) })}
            </p>
            <div className="ed-row">
              <button
                type="button"
                disabled={readOnly || !bg}
                onClick={() => setBackground(null, t("editor.palette.bgRemoved"))}
              >
                {t("editor.palette.bgRemove")}
              </button>
              <button
                type="button"
                disabled={readOnly || !bg || kinds.length < 2}
                onClick={applyToAll}
              >
                {t("editor.palette.bgAll")}
              </button>
            </div>
            {spec && (
              <Swatches
                spec={spec}
                scope={scope}
                value={bg?.color}
                label={t("editor.palette.bgColour")}
                onPick={(c) => setBackground({ color: c }, t("editor.palette.bgSet"))}
              />
            )}
            <h3>{t("editor.palette.bgShapes")}</h3>
            <AssetGrid
              kinds={BG_KINDS}
              label={t("editor.palette.bgShapes")}
              selectedId={bg?.asset}
              disabled={readOnly}
              onPick={(a) => setBackground({ asset: a.id }, t("editor.palette.bgSet"))}
            />
            <h3>{t("editor.palette.tab.images")}</h3>
            <DesignImages
              images={images}
              uploads={uploads}
              readOnly={readOnly}
              selectedId={bg?.media}
              pickLabel={(n, total) => t("editor.palette.bgImageN", { n, total })}
              onPick={(m) => setBackground({ media: m.id }, t("editor.palette.bgSet"))}
            />
          </div>
        )}
      </div>
    </aside>
  );
}
