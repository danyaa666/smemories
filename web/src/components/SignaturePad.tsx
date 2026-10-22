import { useEffect, useId, useRef, useState, type PointerEvent } from "react";
import { useTranslation } from "react-i18next";
import { H, W, addPoint, fit, quantise, type Limits, type Stroke } from "../signature";
import { Button } from "./ui/Button";

const MAX_RAW = 4000; // points kept for one stroke while it is drawn; a longer scribble just stops growing
const PEN = 2.5; // CSS px

/** Draws a stroke smoothed: a quadratic curve through the midpoints of the stored points (display only). */
function trace(ctx: CanvasRenderingContext2D, s: Stroke, k: number) {
  const n = s.length / 2;
  if (n < 1) return;
  ctx.moveTo(s[0]! * k, s[1]! * k);
  if (n === 1) ctx.lineTo(s[0]! * k, s[1]! * k);
  for (let i = 1; i < n - 1; i++) {
    const x = s[2 * i]!;
    const y = s[2 * i + 1]!;
    ctx.quadraticCurveTo(
      x * k,
      y * k,
      ((x + s[2 * i + 2]!) / 2) * k,
      ((y + s[2 * i + 3]!) / 2) * k,
    );
  }
  if (n > 1) ctx.lineTo(s[2 * n - 2]! * k, s[2 * n - 1]! * k);
}

type Props = {
  limits: Limits;
  disabled?: boolean;
  /** The strokes to store (already thinned to the limits), or null when the pad is empty. */
  onChange: (strokes: Stroke[] | null) => void;
};

/**
 * The optional signature pad of the public note form (E11/T-096): one canvas drawn with Pointer Events (finger, pen, mouse),
 * `touch-action: none` on the canvas only. The drawing space is 1000 x 400 whatever the screen size. Only x and y are read from an
 * event: no timing, pressure or pointer type. The canvas is not focusable; Undo and Clear are ordinary buttons.
 */
export function SignaturePad({ limits, disabled, onChange }: Props) {
  const { t } = useTranslation();
  const labelId = useId();
  const hintId = useId();
  const canvas = useRef<HTMLCanvasElement>(null);
  const live = useRef<{ id: number; pts: Stroke } | null>(null);
  const [raw, setRaw] = useState<Stroke[]>([]);
  const [refused, setRefused] = useState(false);
  const [said, setSaid] = useState("");
  const [, setTick] = useState(0);
  const strokes = fit(raw, limits) ?? [];

  const draw = () => {
    const c = canvas.current;
    const ctx = c?.getContext("2d");
    if (!c || !ctx) return;
    const r = c.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const w = Math.round((r.width - 2 * c.clientLeft) * dpr);
    if (w && c.width !== w) {
      c.width = w;
      c.height = Math.round((w * H) / W);
    }
    const k = c.width / W;
    // ink and baseline take the text colour of the canvas, so forced-colors modes (CanvasText) just work
    ctx.clearRect(0, 0, c.width, c.height);
    ctx.strokeStyle = getComputedStyle(c).color;
    ctx.lineCap = ctx.lineJoin = "round";
    ctx.lineWidth = PEN * dpr;
    ctx.globalAlpha = 0.35;
    ctx.setLineDash([2 * dpr, 6 * dpr]);
    ctx.beginPath();
    ctx.moveTo(W * 0.05 * k, H * 0.75 * k);
    ctx.lineTo(W * 0.95 * k, H * 0.75 * k);
    ctx.stroke();
    ctx.globalAlpha = 1;
    ctx.setLineDash([]);
    ctx.beginPath();
    for (const s of live.current ? [...strokes, live.current.pts] : strokes) trace(ctx, s, k);
    ctx.stroke();
  };

  useEffect(draw); // after every render: new strokes, a resize or a language change
  useEffect(() => {
    const c = canvas.current;
    if (!c || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => setTick((n) => n + 1));
    ro.observe(c);
    return () => ro.disconnect();
  }, []);

  const apply = (next: Stroke[], say: string) => {
    setRaw(next);
    setSaid(say);
    onChange(next.length ? fit(next, limits) : null);
  };

  // The drawing space maps to the canvas's content box (inside its border), which is where the ink is painted.
  const at = (e: PointerEvent<HTMLCanvasElement>): [number, number] | null => {
    const c = e.currentTarget;
    const r = c.getBoundingClientRect();
    const left = r.left + c.clientLeft;
    const top = r.top + c.clientTop;
    const width = r.width - 2 * c.clientLeft;
    const height = r.height - 2 * c.clientTop;
    return width > 0 && height > 0
      ? [quantise(e.clientX, left, width, W), quantise(e.clientY, top, height, H)]
      : null;
  };

  const down = (e: PointerEvent<HTMLCanvasElement>) => {
    const p = at(e);
    if (disabled || live.current || !e.isPrimary || e.button > 0 || !p) return;
    if (raw.length >= limits.maxStrokes) return setRefused(true);
    e.currentTarget.setPointerCapture?.(e.pointerId);
    live.current = { id: e.pointerId, pts: [] };
    addPoint(live.current.pts, p[0], p[1]);
    draw();
  };
  const move = (e: PointerEvent<HTMLCanvasElement>) => {
    const p = at(e);
    const l = live.current;
    if (!l || l.id !== e.pointerId || !p || l.pts.length >= MAX_RAW * 2) return;
    if (addPoint(l.pts, p[0], p[1])) draw();
  };
  const up = (e: PointerEvent<HTMLCanvasElement>) => {
    const l = live.current;
    if (!l || l.id !== e.pointerId) return;
    const p = at(e);
    if (p && l.pts.length < MAX_RAW * 2) addPoint(l.pts, p[0], p[1]);
    live.current = null;
    const next = [...raw, l.pts];
    if (fit(next, limits)) {
      setRefused(false);
      apply(next, t("note.signature.added"));
    } else {
      setRefused(true); // the stroke is dropped; what was drawn before stays
      draw();
    }
  };
  const cancel = (e: PointerEvent<HTMLCanvasElement>) => {
    if (live.current?.id !== e.pointerId) return;
    live.current = null; // the browser took the gesture away: drop the half stroke
    draw();
  };

  return (
    <div className="sig-pad" role="group" aria-labelledby={labelId} aria-describedby={hintId}>
      <p id={labelId} className="sig-label">
        {t("note.signature.label")}
      </p>
      <canvas
        ref={canvas}
        className="sig-canvas"
        aria-hidden="true"
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={cancel}
      />
      <small id={hintId} className="field-help">
        {t("note.signature.hint")}
      </small>
      <div className="sig-actions">
        <Button
          disabled={disabled || raw.length === 0}
          onClick={() => {
            setRefused(false);
            apply(raw.slice(0, -1), raw.length > 1 ? "" : t("note.signature.cleared"));
          }}
        >
          {t("note.signature.undo")}
        </Button>
        <Button
          disabled={disabled || raw.length === 0}
          onClick={() => {
            setRefused(false);
            apply([], t("note.signature.cleared"));
          }}
        >
          {t("note.signature.clear")}
        </Button>
      </div>
      {refused && (
        <p role="alert" className="field-error">
          {t("note.signature.tooDetailed")}
        </p>
      )}
      <p role="status" className="visually-hidden">
        {said}
      </p>
    </div>
  );
}
