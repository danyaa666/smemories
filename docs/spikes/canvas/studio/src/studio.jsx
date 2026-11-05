// The studio: Fabric.js canvas, images only. Loaded with import() from shell.jsx. Exposes window.__studio for the tests.
import { Canvas, FabricImage, Rect, Ellipse, PencilBrush } from "fabric";
import { useEffect, useRef, useState } from "react";
import { flattenSize, toBlob, PAPERS } from "./flatten.mjs";

const DRAFT = (book) => `studio-draft:${book}`;
export default function Studio({ book, paper, widthMm, upload, onDone, onClose }) {
  const el = useRef(null), cv = useRef(null), busy = useRef(false);
  const [msg, setMsg] = useState(""), [dirty, setDirty] = useState(false), [crash, setCrash] = useState(false), [wait, setWait] = useState(0);
  if (crash) throw new Error("studio crashed (test)");
  useEffect(() => {
    const ratio = PAPERS[paper], w = 420;
    const c = new Canvas(el.current, { width: w, height: Math.round(w * ratio), backgroundColor: "#ffffff" });
    cv.current = c;
    const raw = localStorage.getItem(DRAFT(book));
    if (raw) c.loadFromJSON(JSON.parse(raw)).then(() => { c.renderAll(); setMsg("draft restored"); });
    const mark = () => { setDirty(true); try { localStorage.setItem(DRAFT(book), JSON.stringify(c.toJSON())); } catch { /* quota: the draft is best effort */ } };
    c.on("object:added", mark); c.on("object:modified", mark);
    window.__studio = { canvas: c, addRect: () => c.add(new Rect({ left: 40, top: 40, width: 120, height: 80, fill: "#d9668c" })), addEllipse: () => c.add(new Ellipse({ left: 200, top: 120, rx: 60, ry: 40, fill: "#8e6ccb" })), crash: () => setCrash(true) };
    return () => { c.dispose(); delete window.__studio; };
  }, [book, paper]);
  useEffect(() => { // leaving with unsaved work: the browser asks; the draft is in localStorage anyway
    const f = (e) => { if (dirty) { e.preventDefault(); e.returnValue = ""; } };
    window.addEventListener("beforeunload", f); return () => window.removeEventListener("beforeunload", f);
  }, [dirty]);
  async function save() {
    if (busy.current) return; // double click: one upload
    busy.current = true; setMsg("saving");
    try {
      const { w, h } = flattenSize(paper, widthMm), c = cv.current;
      const blob = await toBlob(c.toCanvasElement(w / c.getWidth()), "image/png"); // multiplier: the flatten size over the working size
      const r = await upload(blob);
      if (r.ok) { localStorage.removeItem(DRAFT(book)); setDirty(false); setMsg("saved"); onDone?.(r.id); }
      else if (r.code === "ERROR_RATE_LIMITED") { const s = r.retryAfter ?? 5; setMsg(`rate limited, retry in ${s}s`); setWait(s); setTimeout(() => setWait(0), s * 1000); }
      else if (r.code === "ERROR_DESIGN_IMAGE_LIMIT") setMsg("design image limit (30) reached: delete one first");
      else if (r.code === "ERROR_QUOTA_EXCEEDED") setMsg("storage quota exceeded");
      else setMsg("upload failed, your draft is kept");
    } catch (e) { setMsg(e.message === "export_failed" ? "export failed (memory), your draft is kept" : "upload failed, your draft is kept"); }
    finally { busy.current = false; }
  }
  return (
    <section aria-label="studio">
      <canvas ref={el} />
      <button onClick={() => window.__studio.addRect()}>rect</button>
      <button id="save" disabled={wait > 0} onClick={save}>save as image</button>
      <button onClick={onClose}>close</button>
      <p role="status" id="msg">{msg}</p>
    </section>
  );
}
