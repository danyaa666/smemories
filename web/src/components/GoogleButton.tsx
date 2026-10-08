// Shown only when the build says Google sign-in is configured (VITE_GOOGLE_SIGNIN=true).
// A plain link: the browser must navigate, not fetch, because the API answers with a redirect.
export function GoogleButton({ label, returnTo }: { label: string; returnTo: string }) {
  if (import.meta.env.VITE_GOOGLE_SIGNIN !== "true") return null;
  return (
    <a
      className="button"
      href={`/api/v1/auth/google/start?return_to=${encodeURIComponent(returnTo)}`}
    >
      {label}
    </a>
  );
}
