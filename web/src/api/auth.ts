import type { components } from "./schema";
import { ApiError, postJson, request } from "./client";

export type User = components["schemas"]["User"];

/** The signed-in user, or null when there is no valid session. */
export async function getMe(): Promise<User | null> {
  try {
    return (await request<components["schemas"]["UserEnvelope"]>("/v1/me")).user;
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null;
    throw e;
  }
}

const user = async (path: string, body: unknown) =>
  (await postJson<components["schemas"]["UserEnvelope"]>(path, body)).user;

export const register = (email: string, password: string, display_name: string) =>
  user("/v1/auth/register", { email, password, display_name });
export const login = (email: string, password: string) =>
  user("/v1/auth/login", { email, password });
export const logout = () => postJson<void>("/v1/auth/logout");
export const verifyEmail = (token: string) => postJson<void>("/v1/auth/verify-email", { token });
export const resendVerification = () =>
  postJson<{ already_verified?: true }>("/v1/auth/verify-email/resend");
export const forgotPassword = (email: string) =>
  postJson<void>("/v1/auth/forgot-password", { email });
export const resetPassword = (token: string, password: string) =>
  postJson<void>("/v1/auth/reset-password", { token, password });
