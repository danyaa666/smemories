import type { components } from "./schema";
import { ApiError, postJson, request } from "./client";

export type User = components["schemas"]["User"];

/** The signed-in user, or null when there is no valid session. */
export async function getMe(): Promise<User | null> {
  try {
    return await request<User>("/user/get-me");
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
) => postJson<User>("/auth/register", { email, password, display_name, locale });
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
