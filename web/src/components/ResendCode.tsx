import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

export const RESEND_COOLDOWN_S = 60;

// Only the "next allowed at" time is kept, never the code or the email; a stale value is ignored.
const storedUntil = (key: string) => {
  try {
    const v = Number(sessionStorage.getItem(key));
    return v > Date.now() ? v : 0;
  } catch {
    return 0; // storage blocked: the cooldown just does not survive a reload
  }
};

/** "Send a new code": disabled with a countdown for a minute after each send (and after arriving from a send). */
export function ResendCode({
  onSend,
  pending,
  startCooling,
  storageKey,
}: {
  onSend: () => Promise<boolean>; // true when a code went out
  pending: boolean;
  startCooling: boolean;
  storageKey: string; // one per screen
}) {
  const { t } = useTranslation();
  // An end time, not a decrementing counter: a throttled background tab still unlocks on time.
  const [until, setUntil] = useState(
    () => storedUntil(storageKey) || (startCooling ? Date.now() + RESEND_COOLDOWN_S * 1000 : 0),
  );
  useEffect(() => {
    try {
      if (until > Date.now()) sessionStorage.setItem(storageKey, String(until));
    } catch {
      // see storedUntil
    }
  }, [until, storageKey]);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (until <= Date.now()) return;
    const id = setInterval(() => {
      setNow(Date.now());
      if (Date.now() >= until) clearInterval(id);
    }, 1000);
    return () => clearInterval(id);
  }, [until]);
  const left = Math.max(0, Math.ceil((until - now) / 1000));
  return (
    <button
      type="button"
      className="resend"
      disabled={pending || left > 0}
      onClick={() => {
        void onSend().then(
          (sent) => sent && (setNow(Date.now()), setUntil(Date.now() + RESEND_COOLDOWN_S * 1000)),
        );
      }}
    >
      {left > 0 ? t("code.resendIn", { seconds: left }) : t("code.resend")}
    </button>
  );
}
