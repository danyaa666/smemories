import { describe, expect, it } from "vitest";
import { GUIDE_STEPS, detectGuide, type GuideId } from "./guide";

const UA = {
  chromeWin:
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  chromeMac:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
  edgeWin:
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0",
  safariMac:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
  firefoxWin: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
  firefoxLinux: "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
  chromeAndroid:
    "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Mobile Safari/537.36",
  samsungAndroid:
    "Mozilla/5.0 (Linux; Android 14; SAMSUNG SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/121.0.0.0 Mobile Safari/537.36",
  firefoxAndroid: "Mozilla/5.0 (Android 14; Mobile; rv:128.0) Gecko/128.0 Firefox/128.0",
  safariIphone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
  chromeIphone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/127.0.0.0 Mobile/15E148 Safari/604.1",
  firefoxIphone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/128.0 Mobile/15E148 Safari/605.1.15",
  safariIpad:
    "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
  ipadDesktopMode:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
};

describe("detectGuide", () => {
  it.each<[string, string, GuideId, number?]>([
    ["Chrome on Windows", UA.chromeWin, "chromium_desktop"],
    ["Chrome on a Mac", UA.chromeMac, "chromium_desktop"],
    ["Edge on Windows", UA.edgeWin, "chromium_desktop"],
    ["Safari on a Mac", UA.safariMac, "safari_desktop"],
    ["a Mac with no touch screen", UA.ipadDesktopMode, "safari_desktop", 0],
    ["Firefox on Windows", UA.firefoxWin, "firefox"],
    ["Firefox on Linux", UA.firefoxLinux, "firefox"],
    ["Chrome on Android", UA.chromeAndroid, "chrome_android"],
    ["Samsung Internet (says Chrome)", UA.samsungAndroid, "chrome_android"],
    ["Firefox on Android (no guide of its own)", UA.firefoxAndroid, "other"],
    ["Safari on an iPhone", UA.safariIphone, "safari_ios"],
    ["Chrome on an iPhone (WebKit)", UA.chromeIphone, "safari_ios"],
    ["Firefox on an iPhone (WebKit)", UA.firefoxIphone, "safari_ios"],
    ["Safari on an iPad", UA.safariIpad, "safari_ios"],
    ["an iPad that says Macintosh (touch screen)", UA.ipadDesktopMode, "safari_ios", 5],
    ["nothing", "", "other"],
    ["a crawler", "Googlebot/2.1 (+http://www.google.com/bot.html)", "other"],
    ["a text browser", "Lynx/2.9.0 libwww-FM/2.14", "other"],
  ])("%s", (_name, userAgent, want, maxTouchPoints) => {
    expect(
      detectGuide({ userAgent, ...(maxTouchPoints === undefined ? {} : { maxTouchPoints }) }),
    ).toBe(want);
  });

  it("has steps for every guide", () => {
    expect(Object.keys(GUIDE_STEPS).sort()).toEqual(
      [
        "chrome_android",
        "chromium_desktop",
        "firefox",
        "other",
        "safari_desktop",
        "safari_ios",
      ].sort(),
    );
  });
});
