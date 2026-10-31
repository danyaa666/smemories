// Question 3: a text box that is part of the printed page, edited in place. Two techniques, same box, same CSS, same fit rule as the printed page.
import { useLayoutEffect, useRef, useState } from "react";
import { fitElement } from "@web/book/fit";
import { Sheet } from "./book.jsx";
import { TEXT_CSS } from "./items.jsx";

const MAX_RUNES = 500;
const START = TEXT_CSS.fontSize; // px, the box's own size; shrinks to 50% at most, then clips (the guided templates' rule, fit.tsx)
export const cap = (s) => Array.from(s).slice(0, MAX_RUNES).join("");

/** The printed text: what the book draws. Fit rule = fitElement of the book, re-run when web fonts finish loading (as web/src/book/fit.tsx FitText does). */
function View({ value, hidden, onFit }) {
  const ref = useRef(null);
  useLayoutEffect(() => {
    const el = ref.current;
    let live = true;
    const fit = () => {
      if (!live) return;
      const info = fitElement(el, `${START}px`, 0.5);
      el.dataset.fit = info.state;
      onFit?.(info);
    };
    fit();
    document.fonts.ready.then(fit);
    document.fonts.addEventListener("loadingdone", fit);
    return () => { live = false; document.fonts.removeEventListener("loadingdone", fit); };
  }, [value, onFit]);
  return <div ref={ref} data-print-text style={{ ...TEXT_CSS, width: "100%", height: "100%", overflow: "hidden", visibility: hidden ? "hidden" : "visible" }}>{value}</div>;
}

const insertPlain = (e, set) => {
  e.preventDefault();
  const text = e.clipboardData.getData("text/plain").replace(/\r\n?/g, "\n");
  document.execCommand("insertText", false, text); // keeps the browser's own undo and caret
  set?.();
};

/** A: a textarea laid exactly over the printed text (same box, same CSS, the size the fit rule chose). */
function Textarea({ value, setValue, px, log }) {
  const composing = useRef(false);
  const ta = useRef(null);
  const caret = useRef(null); // where the caret was when the cap rewrote the value (a controlled textarea would put it at the end)
  useLayoutEffect(() => {
    if (caret.current !== null && ta.current) { ta.current.setSelectionRange(caret.current, caret.current); caret.current = null; }
  }, [value]);
  const set = (el, next) => { if (next !== el.value) caret.current = Math.min(el.selectionStart, next.length); setValue(next); };
  // overflow stays "hidden" in every fit state (0 px against print, also rotated): a hidden textarea still scrolls to the caret while typing (text.json clip_state_typing_at_end);
  // the mouse wheel does not scroll it. An "overflow" box (text cut) is also warned about by the editor, not by the field.
  const params = new URLSearchParams(location.search);
  return (
    <textarea
      ref={ta}
      data-edit="ta" aria-label="Text" value={value} autoFocus
      onChange={(e) => (composing.current ? setValue(e.target.value) : set(e.target, cap(e.target.value)))}
      onCompositionStart={(e) => { composing.current = true; log(e); }}
      onCompositionUpdate={log}
      onCompositionEnd={(e) => { composing.current = false; set(e.target, cap(e.target.value)); log(e); }}
      onPaste={(e) => insertPlain(e)}
      style={{ ...TEXT_CSS, fontSize: px, position: "absolute", inset: 0, width: "100%", height: "100%", border: 0, outline: "1px dashed #8e6ccb", background: "transparent", resize: "none", overflow: "hidden",
        caretColor: params.get("caret") === "off" ? "transparent" : "#3b2f4a" }}
    />
  );
}

/** B: the printed element itself made editable (contenteditable="plaintext-only"), fitted by the same rule. */
function Editable({ value, setValue, log }) {
  const ref = useRef(null);
  const composing = useRef(false);
  useLayoutEffect(() => { ref.current.innerText = value; fitElement(ref.current, `${START}px`, 0.5); ref.current.focus(); }, []); // eslint-disable-line
  const sync = () => {
    const el = ref.current;
    const text = el.innerText.replace(/\n$/, "");
    if (!composing.current && Array.from(text).length > MAX_RUNES) {
      el.innerText = cap(text); // the caret jumps to the start here: this is the technique's weak spot
      const r = document.createRange(); r.selectNodeContents(el); r.collapse(false);
      getSelection().removeAllRanges(); getSelection().addRange(r);
    }
    fitElement(el, `${START}px`, 0.5);
    setValue(cap(text));
  };
  return (
    <div
      ref={ref} data-edit="ce" contentEditable="plaintext-only" role="textbox" aria-multiline="true" suppressContentEditableWarning
      onInput={sync}
      onCompositionStart={(e) => { composing.current = true; log(e); }}
      onCompositionUpdate={log}
      onCompositionEnd={(e) => { composing.current = false; log(e); sync(); }}
      onPaste={(e) => insertPlain(e, sync)}
      style={{ ...TEXT_CSS, width: "100%", height: "100%", overflow: "hidden", outline: "1px dashed #8e6ccb", caretColor: "transparent" }}
    />
  );
}

export function TextLab() {
  const q = new URLSearchParams(location.search);
  const tech = q.get("tech") ?? "ta";
  const mode = q.get("mode") ?? "view";
  const size = q.get("size") ?? "A5";
  const rot = Number(q.get("r") ?? 0);
  const [value, setValue] = useState(q.get("text") ?? "Việt Nam — đường về nhà");
  const w = Number(q.get("w") ?? 440), h = Number(q.get("h") ?? 200);
  const [px, setPx] = useState(START);
  const events = useRef([]);
  window.__text = () => value;
  window.__events = () => events.current;
  const log = (e) => events.current.push(`${e.type}${e.data !== undefined && e.data !== null ? `:${e.data}` : ""}`);
  return (
    <Sheet size={size}>
      <div style={{ position: "absolute", left: 80, top: 330, width: w, height: h, transform: rot ? `rotate(${rot}deg)` : undefined }} data-box>
        <View value={value} hidden={mode === "edit" && tech === "ta"} onFit={(i) => setPx(i.px)} />
        {mode === "edit" && tech === "ta" && <Textarea value={value} setValue={setValue} px={px} log={log} />}
        {mode === "edit" && tech === "ce" && <div style={{ position: "absolute", inset: 0, background: "transparent" }}><Editable value={value} setValue={setValue} log={log} /></div>}
      </div>
    </Sheet>
  );
}
