"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";
import { CheckCircle2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { useResendVerification, useVerifyEmail } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";

/** Lands here from the link auth.Service.Signup emails (verify-email?token=…).
 * No separate "preview" step first (auth.Service.VerifyEmail's own doc
 * comment: nothing to show before consuming — no name/role choice the
 * way accept-invite has) — same loading/invalid/success shape as
 * accept-invite-form.tsx, just via useVerifyEmail (a useQuery, see its
 * own doc comment) instead of usePreviewInvite. */
export function VerifyEmailPanel({ token }: { token: string }) {
  const { t } = useTranslation("auth");
  const router = useRouter();
  const verifyEmail = useVerifyEmail(token);
  const resend = useResendVerification();
  const [resendEmail, setResendEmail] = React.useState("");

  React.useEffect(() => {
    if (verifyEmail.data?.loggedIn) {
      const id = setTimeout(() => router.replace("/overview"), 1200);
      return () => clearTimeout(id);
    }
  }, [verifyEmail.data, router]);

  if (verifyEmail.isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardDescription>{t("verifyEmail.loading")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </CardContent>
      </Card>
    );
  }

  if (!token || !verifyEmail.data) {
    const error = verifyEmail.error instanceof ApiError ? verifyEmail.error : null;
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("verifyEmail.invalidTitle")}</CardTitle>
          <CardDescription>{error?.message ?? t("verifyEmail.invalidDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {resend.isSuccess ? (
            <Alert>
              <AlertDescription>{t("signup.checkEmail.resendSuccess")}</AlertDescription>
            </Alert>
          ) : (
            <>
              <div className="grid gap-1.5">
                <Label htmlFor="verify-resend-email">{t("verifyEmail.resendEmailLabel")}</Label>
                <Input
                  id="verify-resend-email"
                  type="email"
                  autoComplete="email"
                  value={resendEmail}
                  onChange={(e) => setResendEmail(e.target.value)}
                />
              </div>
              <Button disabled={!resendEmail || resend.isPending} onClick={() => resend.mutate(resendEmail)}>
                {t("signup.checkEmail.resendButton")}
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="items-center text-center">
        <CheckCircle2Icon className="mb-2 size-8 text-success" aria-hidden="true" />
        <CardTitle>{t("verifyEmail.successTitle")}</CardTitle>
        <CardDescription>
          {verifyEmail.data.loggedIn ? t("verifyEmail.successDescriptionLoggedIn") : t("verifyEmail.successDescriptionLoginRequired")}
        </CardDescription>
      </CardHeader>
      {!verifyEmail.data.loggedIn && (
        <CardContent>
          <Button className="w-full" onClick={() => router.push("/login")}>
            {t("verifyEmail.goToLoginButton")}
          </Button>
        </CardContent>
      )}
    </Card>
  );
}
