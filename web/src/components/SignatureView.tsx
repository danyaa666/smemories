import { signaturePath } from "../signature";

/**
 * A stored signature as inline SVG. The path is built only from validated integers (signaturePath); a value that fails the
 * checks renders nothing. No markup, no image URL.
 */
export function SignatureView({ value, label }: { value: unknown; label: string }) {
  const d = signaturePath(value);
  if (!d) return null;
  return (
    <svg className="sig-view" viewBox="0 0 1000 400" role="img" aria-label={label}>
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
