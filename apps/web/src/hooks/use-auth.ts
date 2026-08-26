"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

import {
  acceptInvite,
  getMe,
  login,
  logout,
  previewInvite,
  signup,
  type AcceptInviteInput,
  type LoginInput,
  type SignupInput,
} from "@/lib/api/auth";
import { ApiError } from "@/lib/api/client";

const meKey = ["auth", "me"] as const;

/** 401 ("not signed in") is a normal, common state here, not an error to
 * surface via ErrorState — every caller should check `query.data`
 * (undefined when signed out) rather than `query.isError`, same pattern
 * as use-ad-account-connection.ts's own 404-is-normal handling. `retry:
 * false` matters here specifically: TanStack Query's default retries a
 * failed query with backoff, which would turn "briefly show the login
 * page" into "spend several seconds retrying a 401 first." */
export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await getMe();
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null;
        throw err;
      }
    },
    retry: false,
    staleTime: 60_000,
  });
}

export function useSignup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SignupInput) => signup(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LoginInput) => login(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => logout(),
    // Every other cached query (campaigns, team members, ...) belongs to
    // the session that just ended — clear() rather than invalidate() so
    // the next signed-in user (even the same one, in a fresh session)
    // never renders a flash of the previous session's cached data before
    // its own queries refetch.
    onSuccess: () => qc.clear(),
  });
}

export function usePreviewInvite(token: string) {
  return useQuery({
    queryKey: ["auth", "invite-preview", token],
    queryFn: () => previewInvite(token),
    enabled: !!token,
    retry: false,
  });
}

export function useAcceptInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AcceptInviteInput) => acceptInvite(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  });
}
