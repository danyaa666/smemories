/**
 * Which print guide to show. The page cannot set the paper size, margins or background graphics of the browser's print dialog
 * (ADR 0003), so the owner is told what to choose, per browser. The ids are also the `browser_family` of the feedback counter
 * (api/openapi.yaml), so a failing device class shows up in the counts.
 */
export type GuideId =
  "chromium_desktop" | "safari_desktop" | "firefox" | "chrome_android" | "safari_ios" | "other";

type Navigator = { userAgent: string; maxTouchPoints?: number };

/**
 * Picks the guide from the user agent. Order matters: every browser on an iPhone or iPad is WebKit and prints through the
 * share sheet (an iPad in desktop mode says Macintosh, but has a touch screen); Chrome, Edge, Samsung Internet and Opera all say
 * "Chrome/" and share the Chromium print dialog; Chrome says "Safari/" too, so Safari is tested last and needs "Version/".
 * Anything this does not recognise gets the generic steps (`other`).
 */
export function detectGuide({ userAgent: ua, maxTouchPoints = 0 }: Navigator): GuideId {
  if (/iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && maxTouchPoints > 1))
    return "safari_ios";
  if (/Android/.test(ua)) return /Chrome\//.test(ua) ? "chrome_android" : "other";
  if (/Firefox\//.test(ua)) return "firefox";
  if (/Edg\/|Chrome\/|Chromium\//.test(ua)) return "chromium_desktop";
  if (/Safari\//.test(ua) && /Version\//.test(ua)) return "safari_desktop";
  return "other";
}

/** How many numbered steps each guide has (locale keys `print.guide.<id>.step1` ...). */
export const GUIDE_STEPS: Record<GuideId, number> = {
  chromium_desktop: 5,
  safari_desktop: 5,
  firefox: 5,
  chrome_android: 5,
  safari_ios: 4,
  other: 4,
};
