"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

import {
  acceptInvite,
  getMe,
  login,
  logout,
  previewInvite,
  resendVerification,
  signup,
  verifyEmail,
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

// No meKey invalidation on success — signup no longer creates a session
// (see lib/api/auth.ts's SignupResponse doc comment), so "who am I" is
// unchanged by it. useVerifyEmail below is the one that can actually
// start a session.
export function useSignup() {
  return useMutation({
    mutationFn: (input: SignupInput) => signup(input),
  });
}

// useQuery, not useMutation — despite the underlying request being a POST
// (it consumes a one-time token), the shape this needs is exactly
// usePreviewInvite's below: "fetch once for this token, on mount, cache
// against it" — no manual useEffect+useRef mount-guard required, React
// Query's own enabled+queryKey semantics already do that. staleTime:
// Infinity because the token is single-use — a refetch would just 404
// against an already-consumed one, never anything worth re-requesting.
export function useVerifyEmail(token: string) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: ["auth", "verify-email", token],
    queryFn: async () => {
      const res = await verifyEmail(token);
      if (res.loggedIn) qc.invalidateQueries({ queryKey: meKey });
      return res;
    },
    enabled: !!token,
    retry: false,
    staleTime: Infinity,
  });
}

export function useResendVerification() {
  return useMutation({
    mutationFn: (email: string) => resendVerification(email),
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
