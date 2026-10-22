import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";
import { loaded } from "../components/imageQueue";
import i18n from "../i18n";

beforeEach(async () => {
  window.localStorage.clear();
  await i18n.changeLanguage("en");
});

afterEach(() => {
  cleanup();
  loaded.clear(); // the set of images already fetched outlives a component
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
