"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { MailIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useResendVerification, useSignup } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";

function buildSignupSchema(t: TFunction) {
  return z.object({
    organizationName: z.string().min(1, t("signup.validation.organizationNameRequired", { ns: "auth" })).max(120),
    name: z.string().min(1, t("signup.validation.nameRequired", { ns: "auth" })).max(120),
    email: z.email(t("signup.validation.emailInvalid", { ns: "auth" })),
    password: z.string().min(8, t("signup.validation.passwordMin", { ns: "auth" })),
  });
}

type SignupFormValues = z.infer<ReturnType<typeof buildSignupSchema>>;

export function SignupForm() {
  const { t } = useTranslation("auth");
  const signup = useSignup();
  // §30.7: a referral link is https://flox.io/r/<code>-shaped, but the
  // actual signup landing page here is this app's own /signup — a real
  // referral link points here with the code as a plain ?ref= query param
  // instead. Read once, silently, at mount (same pattern analytics-view.tsx
  // already uses for its own dim/val/tab params) — no visible field, no
  // validation client-side; an invalid/unknown code is a silent no-op on
  // the real backend, never something worth surfacing as a form error.
  const searchParams = useSearchParams();
  const referralCode = searchParams.get("ref") ?? undefined;
  // Signup no longer logs in — see useSignup's own doc comment. Once it
  // succeeds, this swaps the form out for a "check your email" panel
  // (below) instead of redirecting into the app; email is only used
  // there, so a plain ref-free string state is enough.
  const [verifyEmailAddress, setVerifyEmailAddress] = React.useState<string | null>(null);

  const form = useForm<SignupFormValues>({
    resolver: zodResolver(buildSignupSchema(t)),
    defaultValues: { organizationName: "", name: "", email: "", password: "" },
  });

  function onSubmit(values: SignupFormValues) {
    signup.mutate(
      { ...values, referralCode },
      { onSuccess: (res) => setVerifyEmailAddress(res.user.email) },
    );
  }

  const error = signup.error instanceof ApiError ? signup.error : null;

  if (verifyEmailAddress) {
    return <CheckEmailPanel email={verifyEmailAddress} />;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("signup.title")}</CardTitle>
        <CardDescription>{t("signup.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="signup-org">{t("signup.organizationNameLabel")}</Label>
            <Input
              id="signup-org"
              placeholder={t("signup.organizationNamePlaceholder")}
              autoFocus
              {...form.register("organizationName")}
              aria-invalid={!!form.formState.errors.organizationName}
            />
            {form.formState.errors.organizationName && (
              <p className="text-xs text-danger">{form.formState.errors.organizationName.message}</p>
            )}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="signup-name">{t("signup.nameLabel")}</Label>
            <Input
              id="signup-name"
              placeholder={t("signup.namePlaceholder")}
              autoComplete="name"
              {...form.register("name")}
              aria-invalid={!!form.formState.errors.name}
            />
            {form.formState.errors.name && <p className="text-xs text-danger">{form.formState.errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="signup-email">{t("signup.emailLabel")}</Label>
            <Input
              id="signup-email"
              type="email"
              autoComplete="email"
              {...form.register("email")}
              aria-invalid={!!form.formState.errors.email}
            />
            {form.formState.errors.email && <p className="text-xs text-danger">{form.formState.errors.email.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="signup-password">{t("signup.passwordLabel")}</Label>
            <Input
              id="signup-password"
              type="password"
              autoComplete="new-password"
              {...form.register("password")}
              aria-invalid={!!form.formState.errors.password}
            />
            {form.formState.errors.password ? (
              <p className="text-xs text-danger">{form.formState.errors.password.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">{t("signup.passwordHint")}</p>
            )}
          </div>

          <Button type="submit" className="w-full" disabled={signup.isPending}>
            {t("signup.submitButton")}
          </Button>

          <p className="text-center text-sm text-muted-foreground">
            {t("signup.hasAccount")}{" "}
            <Link href="/login" className="font-medium text-foreground underline underline-offset-4">
              {t("signup.loginLink")}
            </Link>
          </p>
        </form>
      </CardContent>
    </Card>
  );
}

/** Post-signup success state — an org+account now exists, but there's no
 * session yet (see useSignup's own doc comment), so this replaces the
 * form rather than the usual "success toast + redirect" this app uses
 * elsewhere: there's genuinely nowhere in the app to redirect into until
 * the emailed link is clicked. */
function CheckEmailPanel({ email }: { email: string }) {
  const { t } = useTranslation("auth");
  const resend = useResendVerification();

  return (
    <Card>
      <CardHeader className="items-center text-center">
        <MailIcon className="mb-2 size-8 text-muted-foreground" aria-hidden="true" />
        <CardTitle>{t("signup.checkEmail.title")}</CardTitle>
        <CardDescription>{t("signup.checkEmail.description", { email })}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col items-center gap-3">
        {resend.isSuccess ? (
          <Alert>
            <AlertDescription>{t("signup.checkEmail.resendSuccess")}</AlertDescription>
          </Alert>
        ) : (
          <Button variant="outline" onClick={() => resend.mutate(email)} disabled={resend.isPending}>
            {t("signup.checkEmail.resendButton")}
          </Button>
        )}
        <Link href="/login" className="text-sm font-medium text-foreground underline underline-offset-4">
          {t("signup.loginLink")}
        </Link>
      </CardContent>
    </Card>
  );
}
