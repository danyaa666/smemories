import { buttonClass } from "./ui/Button";

// Shown only when the build says Google sign-in is configured (VITE_GOOGLE_SIGNIN=true).
// A plain link: the browser must navigate, not fetch, because the API answers with a redirect.
export const googleEnabled = () => import.meta.env.VITE_GOOGLE_SIGNIN === "true";

export function GoogleButton({ label, returnTo }: { label: string; returnTo: string }) {
  if (!googleEnabled()) return null;
  return (
    <a
      className={buttonClass("secondary")}
      href={`/api/auth/google-start?return_to=${encodeURIComponent(returnTo)}`}
    >
      {label}
    </a>
  );
}
