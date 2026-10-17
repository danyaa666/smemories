import { useMutation } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { sendPrintFeedback } from "../api/templates";
import { errorText } from "../auth/errors";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { GUIDE_STEPS, detectGuide, type GuideId } from "./guide";

/** The guide of this browser, from its user agent. */
export const useGuide = (): GuideId =>
  useMemo(
    () => detectGuide({ userAgent: navigator.userAgent, maxTouchPoints: navigator.maxTouchPoints }),
    [],
  );

/**
 * The exact print-dialog settings for this browser (they cannot be set from the page, so the guide is the control), then the
 * "I printed with these settings" link, which opens the anonymous "Does the PDF look right?" question. The question also opens
 * by itself when the browser reports the end of printing (`afterprint`; iOS Safari does not always send it, hence the link).
 */
export function PrintGuide({ size }: { size: string }) {
  const { t } = useTranslation();
  const guide = useGuide();
  const [asking, setAsking] = useState<"" | "auto" | "link">("");
  useEffect(() => {
    const after = () => setAsking((a) => a || "auto");
    window.addEventListener("afterprint", after);
    return () => window.removeEventListener("afterprint", after);
  }, []);
  const steps = Array.from({ length: GUIDE_STEPS[guide] }, (_, i) => `step${i + 1}`);
  return (
    <section className="bk-guide" aria-labelledby="bk-guide-title">
      <h2 id="bk-guide-title">{t("print.guide.title")}</h2>
      <p>{t("print.guide.device", { name: t(`print.guide.${guide}.name`) })}</p>
      <p>{t("print.guide.cannotCheck")}</p>
      <ol>
        {steps.map((s) => (
          <li key={s}>{t(`print.guide.${guide}.${s}`, { size })}</li>
        ))}
      </ol>
      {asking === "" && (
        <Button variant="ghost" onClick={() => setAsking("link")}>
          {t("print.guide.printed")}
        </Button>
      )}
      {asking !== "" && <Feedback family={guide} focus={asking === "link"} />}
    </section>
  );
}

/** One yes or no, sent once. A 4xx is the answer (no retry); the buttons stay so a failed send can be tried again. */
function Feedback({ family, focus }: { family: GuideId; focus: boolean }) {
  const { t } = useTranslation();
  const head = useRef<HTMLHeadingElement>(null);
  const done = useRef<HTMLParagraphElement>(null);
  const send = useMutation({
    mutationFn: (verdict: "yes" | "no") => sendPrintFeedback({ verdict, browser_family: family }),
  });
  useEffect(() => {
    if (focus) head.current?.focus(); // the owner asked for it: move to the question
  }, [focus]);
  useEffect(() => {
    if (send.isSuccess) done.current?.focus();
  }, [send.isSuccess]);
  return (
    <div className="bk-feedback">
      <h3 ref={head} tabIndex={-1}>
        {t("print.feedback.title")}
      </h3>
      {send.isSuccess ? (
        <p ref={done} tabIndex={-1} role="status">
          {t(send.variables === "yes" ? "print.feedback.thanksYes" : "print.feedback.thanksNo")}
        </p>
      ) : (
        <>
          <p>{t("print.feedback.hint")}</p>
          <div className="bk-feedback-buttons">
            {/* not disabled while sending: the focus stays on the button; a second click is ignored */}
            <Button
              aria-busy={send.isPending}
              onClick={() => !send.isPending && send.mutate("yes")}
            >
              {t("print.feedback.yes")}
            </Button>
            <Button aria-busy={send.isPending} onClick={() => !send.isPending && send.mutate("no")}>
              {t("print.feedback.no")}
            </Button>
          </div>
          {send.isError && <Banner variant="error">{errorText(t, send.error)}</Banner>}
        </>
      )}
    </div>
  );
}
