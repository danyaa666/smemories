// The book editor shell: the book, a feature flag, and the studio behind lazy() + an error boundary. Studio failures stay inside the boundary.
import { Component, lazy, Suspense, useState } from "react";
import { createRoot } from "react-dom/client";
import { Book } from "./book.jsx";

const Studio = lazy(() => import("./studio.jsx"));
class Boundary extends Component {
  state = { err: null };
  static getDerivedStateFromError(err) { return { err }; }
  render() { return this.state.err ? <p role="alert" id="studio-error">The studio could not be used. Your book is safe. <button onClick={() => this.props.onClose()}>close</button></p> : this.props.children; }
}
async function upload(blob) { // the product's POST /api/media/create (purpose=design); the tests answer it
  try {
    const fd = new FormData(); fd.set("yearbook_id", "BOOK"); fd.set("purpose", "design"); fd.set("file", blob, "studio.png");
    const res = await fetch("/api/media/create", { method: "POST", body: fd });
    const j = await res.json().catch(() => ({}));
    return res.ok ? { ok: true, id: j.data?.id } : { ok: false, code: j.status, retryAfter: Number(res.headers.get("Retry-After")) || undefined };
  } catch { return { ok: false, code: "network" }; }
}
function App() {
  const flag = new URLSearchParams(location.search).get("studio") !== "off"; // the feature flag: off = the button is not there
  const [open, setOpen] = useState(false), [paper, setPaper] = useState("A5");
  return (
    <main>
      <Book />
      {flag && <button id="open" onClick={() => setOpen(true)}>Image studio</button>}
      <select id="paper" value={paper} onChange={(e) => setPaper(e.target.value)}><option>A5</option><option>A4</option><option>Letter</option></select>
      {open && <Boundary onClose={() => setOpen(false)}><Suspense fallback={<p>loading studio</p>}><Studio book="BOOK" paper={paper} widthMm={{ A5: 148, A4: 210, Letter: 215.9 }[paper]} upload={upload} onClose={() => setOpen(false)} /></Suspense></Boundary>}
    </main>
  );
}
createRoot(document.getElementById("root")).render(<App />);
