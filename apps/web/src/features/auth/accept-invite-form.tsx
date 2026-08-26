"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { useAcceptInvite, usePreviewInvite } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";

function buildAcceptInviteSchema(t: TFunction) {
  return z.object({
    name: z.string().min(1, t("acceptInvite.validation.nameRequired", { ns: "auth" })).max(120),
    password: z.string().min(8, t("acceptInvite.validation.passwordMin", { ns: "auth" })),
  });
}

type AcceptInviteFormValues = z.infer<ReturnType<typeof buildAcceptInviteSchema>>;

export function AcceptInviteForm({ token }: { token: string }) {
  const { t } = useTranslation("auth");
  const router = useRouter();
  const preview = usePreviewInvite(token);
  const acceptInvite = useAcceptInvite();

  const form = useForm<AcceptInviteFormValues>({
    resolver: zodResolver(buildAcceptInviteSchema(t)),
    defaultValues: { name: "", password: "" },
  });

  // Pre-fills the name field once, the moment the invite preview loads —
  // NOT via useForm's `values` option: that re-applies on every render,
  // and since `preview.data?.email.split("@")[0]` builds a fresh object
  // each time, it would silently wipe out whatever the invitee had
  // already typed (name edits, and the password field entirely) on every
  // keystroke-triggered re-render.
  const prefilled = React.useRef(false);
  React.useEffect(() => {
    if (prefilled.current || !preview.data) return;
    prefilled.current = true;
    form.setValue("name", preview.data.email.split("@")[0]);
  }, [preview.data, form]);

  function onSubmit(values: AcceptInviteFormValues) {
    acceptInvite.mutate(
      { token, name: values.name, password: values.password },
      { onSuccess: () => router.push("/overview") },
    );
  }

  if (preview.isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardDescription>{t("acceptInvite.loading")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </CardContent>
      </Card>
    );
  }

  if (!preview.data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("acceptInvite.invalidTitle")}</CardTitle>
          <CardDescription>{t("acceptInvite.invalidDescription")}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  const acceptError = acceptInvite.error instanceof ApiError ? acceptInvite.error : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("acceptInvite.title", { organizationName: preview.data.organizationName })}</CardTitle>
        <CardDescription>{t("acceptInvite.description", { role: preview.data.role })}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
          {acceptError && (
            <Alert variant="destructive">
              <AlertDescription>{acceptError.message}</AlertDescription>
            </Alert>
          )}

          <div className="grid gap-1.5">
            <Label>{t("acceptInvite.nameLabel")}</Label>
            <Input autoFocus {...form.register("name")} aria-invalid={!!form.formState.errors.name} />
            {form.formState.errors.name && <p className="text-xs text-danger">{form.formState.errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label>{t("acceptInvite.passwordLabel")}</Label>
            <Input
              type="password"
              autoComplete="new-password"
              {...form.register("password")}
              aria-invalid={!!form.formState.errors.password}
            />
            {form.formState.errors.password ? (
              <p className="text-xs text-danger">{form.formState.errors.password.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">{t("acceptInvite.passwordHint")}</p>
            )}
          </div>

          <Button type="submit" className="w-full" disabled={acceptInvite.isPending}>
            {t("acceptInvite.submitButton")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
