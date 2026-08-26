"use client";

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
import { useLogin } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";

function buildLoginSchema(t: TFunction) {
  return z.object({
    email: z.email(t("login.validation.emailInvalid", { ns: "auth" })),
    password: z.string().min(1, t("login.validation.passwordRequired", { ns: "auth" })),
  });
}

type LoginFormValues = z.infer<ReturnType<typeof buildLoginSchema>>;

export function LoginForm() {
  const { t } = useTranslation("auth");
  const router = useRouter();
  const login = useLogin();

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(buildLoginSchema(t)),
    defaultValues: { email: "", password: "" },
  });

  function onSubmit(values: LoginFormValues) {
    login.mutate(values, {
      onSuccess: () => router.push("/overview"),
    });
  }

  const error = login.error instanceof ApiError ? login.error : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("login.title")}</CardTitle>
        <CardDescription>{t("login.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
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
