import { signaturePath } from "../signature";

/** True when `value` is a valid stored signature, i.e. when <BookSignature> draws something (a template reserves room only then). */
export const hasSignature = (value: unknown): boolean => signaturePath(value) !== null;

/**
 * A friend's signature in the printed book (E11, T-097): inline SVG, strokes only, built from validated integers (signaturePath),
 * never from markup. Ink only, no fill and no background, so it survives "background graphics off". The box size comes from the
 * template's class (`className` sets width and height, 5:2 like the drawing space); the stroke is 10 of the 1000 units of its
 * width, so it scales with the box and with the page; the view box is 5 units wider on every side so a stroke at the edge of the drawing space is not clipped. The colour is the template's ink, through `currentColor`. Invalid or null: nothing.
 */
export function BookSignature({
  value,
  label,
  className,
}: {
  value: unknown;
  label: string;
  className: string;
}) {
  const d = signaturePath(value);
  if (!d) return null;
  return (
    <svg className={`bk-sig ${className}`} viewBox="-5 -5 1010 410" role="img" aria-label={label}>
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="10"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
