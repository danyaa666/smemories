import type { components } from "./schema";
import { postJson, request } from "./client";

// `GET /api/template/get-list` and `POST /api/print-feedback/create` (api/openapi.yaml).

type S = components["schemas"];
export type TemplateInfo = S["Template"];
export type PrintFeedback = S["PrintFeedbackCreateRequest"];

/** The templates that support the paper size, ordered by id. */
export const listTemplates = (pageSize: string) =>
  request<S["TemplateListResponse"]["data"]>(
    `/template/get-list?page_size=${encodeURIComponent(pageSize)}`,
  );

/** One anonymous "did the PDF look right?" answer; the server only adds one to a counter. */
export const sendPrintFeedback = (body: PrintFeedback) =>
  postJson<void>("/print-feedback/create", body);
