import type { components } from "./schema";
import { ApiError, postJson, request } from "./client";

export type User = components["schemas"]["User"];
/** What get-me and accept-terms answer: the user plus where they stand on the Terms. login and register answer the plain User, so `terms` may be absent in the cache. */
export type Me = User & Partial<Pick<components["schemas"]["Me"], "terms">>;

/** The signed-in user, or null when there is no valid session. */
export async function getMe(): Promise<Me | null> {
  try {
    return await request<Me>("/user/get-me");
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null;
    throw e;
  }
}

export const register = (
  email: string,
  password: string,
  display_name: string,
  locale: "en" | "vi",
  accept_terms_version: string,
) =>
  postJson<User>("/auth/register", { email, password, display_name, locale, accept_terms_version });
export const acceptTerms = (version: string) => postJson<Me>("/user/accept-terms", { version });
export const login = (email: string, password: string) =>
  postJson<User>("/auth/login", { email, password });
export const logout = () => postJson<void>("/auth/logout");
export const verifyEmail = (code: string) => postJson<void>("/auth/verify-email", { code });
export const resendVerification = () =>
  postJson<components["schemas"]["ResendVerificationResponse"]["data"]>(
    "/auth/resend-verification",
  );
export const forgotPassword = (email: string) => postJson<void>("/auth/forgot-password", { email });
export const resetPassword = (email: string, code: string, password: string) =>
  postJson<void>("/auth/reset-password", { email, code, password });
export const updateUsername = (username: string) =>
  postJson<User>("/user/update-username", { username });
