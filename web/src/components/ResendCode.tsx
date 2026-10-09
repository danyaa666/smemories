import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

export const RESEND_COOLDOWN_S = 60;

/** "Send a new code": disabled with a countdown for a minute after each send (and after arriving from a send). */
export function ResendCode({
  onSend,
  pending,
  startCooling,
}: {
  onSend: () => Promise<boolean>; // true when a code went out
  pending: boolean;
  startCooling: boolean;
}) {
  const { t } = useTranslation();
  // An end time, not a decrementing counter: a throttled background tab still unlocks on time.
  const [until, setUntil] = useState(() =>
    startCooling ? Date.now() + RESEND_COOLDOWN_S * 1000 : 0,
  );
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
