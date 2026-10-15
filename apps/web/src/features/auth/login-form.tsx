"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useLogin, useResendVerification } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";
import { isPendingTOTP, type PendingLogin } from "@/lib/api/auth";
import { TOTPCodeStep, TOTPEnrollStep } from "@/features/auth/totp-step";
import { emailField } from "@/schemas/fields";

function buildLoginSchema(t: TFunction) {
  return z.object({
    email: emailField(t("login.validation.emailInvalid", { ns: "auth" })),
    password: z.string().min(1, t("login.validation.passwordRequired", { ns: "auth" })),
  });
}

type LoginFormValues = z.infer<ReturnType<typeof buildLoginSchema>>;

export function LoginForm() {
  const { t } = useTranslation("auth");
  const router = useRouter();
  const login = useLogin();
  const resend = useResendVerification();
  // §30 Security: a login can resolve to "enter your existing 2FA code"
  // or "the org now requires 2FA, set it up first" instead of a session —
  // both cases replace this form with one of totp-step.tsx's two steps,
  // same "swap the whole card, don't try to layer a second form on top"
  // pattern signup-form.tsx already uses for its own post-submit state.
  const [pending, setPending] = React.useState<PendingLogin | null>(null);

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(buildLoginSchema(t)),
    defaultValues: { email: "", password: "" },
  });

  function onSubmit(values: LoginFormValues) {
    login.mutate(values, {
      onSuccess: (outcome) => {
        if (isPendingTOTP(outcome)) {
          setPending(outcome);
        } else {
          router.push("/overview");
        }
      },
    });
  }

  const error = login.error instanceof ApiError ? login.error : null;
  // apps/internal/auth.Service.Login's distinct "email_not_verified" code
  // (§ its own comment: only revealed once the password is already
  // confirmed correct) gets a "resend the link" CTA instead of the
  // generic error message every other login failure renders as.
  const unverified = error?.code === "email_not_verified";

  if (pending) {
    const onComplete = () => router.push("/overview");
    return pending.requiresTotpEnrollment ? (
      <TOTPEnrollStep pending={pending} onComplete={onComplete} />
    ) : (
      <TOTPCodeStep pending={pending} onComplete={onComplete} />
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("login.title")}</CardTitle>
        <CardDescription>{t("login.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
          {error && !unverified && (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}

          {unverified && (
            <Alert variant="destructive">
              <AlertDescription className="flex flex-col gap-2">
                <span>{error.message}</span>
                {resend.isSuccess ? (
                  <span className="text-foreground">{t("signup.checkEmail.resendSuccess")}</span>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="self-start"
                    disabled={resend.isPending}
                    onClick={() => resend.mutate(form.getValues("email"))}
                  >
                    {t("signup.checkEmail.resendButton")}
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="login-email">{t("login.emailLabel")}</Label>
            <Input
              id="login-email"
              type="email"
              autoComplete="email"
              autoFocus
              {...form.register("email")}
              aria-invalid={!!form.formState.errors.email}
            />
            {form.formState.errors.email && (
              <p className="text-xs text-danger">{form.formState.errors.email.message}</p>
            )}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="login-password">{t("login.passwordLabel")}</Label>
            <Input
              id="login-password"
              type="password"
              autoComplete="current-password"
              {...form.register("password")}
              aria-invalid={!!form.formState.errors.password}
            />
            {form.formState.errors.password && (
              <p className="text-xs text-danger">{form.formState.errors.password.message}</p>
            )}
          </div>

          <Button type="submit" className="w-full" disabled={login.isPending}>
            {t("login.submitButton")}
          </Button>

          <p className="text-center text-sm text-muted-foreground">
            {t("login.noAccount")}{" "}
            <Link href="/signup" className="font-medium text-foreground underline underline-offset-4">
              {t("login.signupLink")}
            </Link>
          </p>
        </form>
      </CardContent>
    </Card>
  );
}
