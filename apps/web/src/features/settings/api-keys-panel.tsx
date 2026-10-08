"use client";

import * as React from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { CopyIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardAction } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Caption, Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useApiKeys, useCreateApiKey, useRevokeApiKey } from "@/hooks/use-api-keys";
import { API_KEY_SCOPES, type ApiKeyScope } from "@/lib/api-keys";

function createKeySchema(t: TFunction<"settings">) {
  return z.object({
    name: z.string().min(2, t("apiKeys.errors.name")).max(60),
    scope: z.enum(API_KEY_SCOPES as [ApiKeyScope, ...ApiKeyScope[]]),
  });
}

type CreateKeyValues = z.infer<ReturnType<typeof createKeySchema>>;

const SCOPE_VARIANT: Record<ApiKeyScope, "outline" | "secondary" | "warning"> = {
  read: "outline",
  write: "secondary",
  admin: "warning",
};

export function ApiKeysPanel() {
  const { t } = useTranslation(["settings", "common"]);
  const apiKeysQuery = useApiKeys();
  const createApiKeyMutation = useCreateApiKey();
  const revokeApiKeyMutation = useRevokeApiKey();
  const apiKeys = apiKeysQuery.data?.apiKeys ?? [];

  const [creating, setCreating] = React.useState(false);
  const [revealedKey, setRevealedKey] = React.useState<string | null>(null);
  const [revokeTarget, setRevokeTarget] = React.useState<{ id: string; name: string } | null>(null);

  const form = useForm<CreateKeyValues>({
    resolver: zodResolver(createKeySchema(t)),
    defaultValues: { name: "", scope: "read" },
  });

  function handleCreate(values: CreateKeyValues) {
    createApiKeyMutation.mutate(values, {
      onSuccess: ({ fullKey }) => {
        setCreating(false);
        form.reset();
        setRevealedKey(fullKey);
      },
      onError: (err) => toast.error(t("apiKeys.createError"), { description: err.message }),
    });
  }

  function copyKey() {
    if (!revealedKey) return;
    navigator.clipboard.writeText(revealedKey);
    toast(t("apiKeys.copied"));
  }

  function handleRevoke() {
    if (!revokeTarget) return;
    revokeApiKeyMutation.mutate(revokeTarget.id, {
      onSuccess: () => toast(t("apiKeys.revoked"), { description: revokeTarget.name }),
      onError: (err) => toast.error(t("apiKeys.revokeError"), { description: err.message }),
    });
    setRevokeTarget(null);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("apiKeys.title")}</CardTitle>
        <CardDescription>{t("apiKeys.description")}</CardDescription>
        <CardAction>
          <Button size="sm" onClick={() => setCreating(true)}>
            <PlusIcon className="size-4" /> {t("apiKeys.new")}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {apiKeysQuery.isPending && (
          <div className="px-4 py-6">
            <LoadingState label={t("apiKeys.loading")} />
          </div>
        )}
        {apiKeysQuery.isError && (
          <div className="px-4 py-6">
            <ErrorState
              title={t("apiKeys.loadError")}
              description={apiKeysQuery.error.message}
              onRetry={() => apiKeysQuery.refetch()}
            />
          </div>
        )}
        {apiKeysQuery.isSuccess && apiKeys.length === 0 && (
          <p className="px-4 py-6 text-sm text-muted-foreground">{t("apiKeys.empty")}</p>
        )}
        {apiKeys.map((key) => (
          <div key={key.id} className="flex items-center gap-3 px-4 py-3">
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium">{key.name}</span>
              <Mono className="text-xs text-muted-foreground">{key.prefix}</Mono>
            </div>
            <Badge variant={SCOPE_VARIANT[key.scope]}>{t(`apiKeys.scopes.${key.scope}`)}</Badge>
            {key.lastUsedAt ? (
              <RelativeTime date={key.lastUsedAt} className="hidden w-32 sm:block" />
            ) : (
              <Caption className="hidden w-32 sm:block">{t("apiKeys.neverUsed")}</Caption>
            )}
            <IconButton
              aria-label={t("apiKeys.revokeAria", { name: key.name })}
              size="icon-sm"
              onClick={() => setRevokeTarget({ id: key.id, name: key.name })}
            >
              <Trash2Icon className="size-3.5" />
            </IconButton>
          </div>
        ))}
      </CardContent>

      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apiKeys.new")}</DialogTitle>
            <DialogDescription>{t("apiKeys.scopeHint")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(handleCreate)} className="flex flex-col gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="key-name">{t("apiKeys.name")}</Label>
              <Input id="key-name" placeholder={t("apiKeys.namePlaceholder")} {...form.register("name")} />
              {form.formState.errors.name && (
                <p className="text-xs text-danger">{form.formState.errors.name.message}</p>
              )}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="key-scope">{t("apiKeys.scope")}</Label>
              <Controller
                control={form.control}
                name="scope"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="key-scope" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {API_KEY_SCOPES.map((s) => (
                        <SelectItem key={s} value={s}>
                          {t(`apiKeys.scopes.${s}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreating(false)}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit">{t("apiKeys.create")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revealedKey} onOpenChange={(open) => !open && setRevealedKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apiKeys.revealTitle")}</DialogTitle>
            <DialogDescription>{t("apiKeys.revealDescription")}</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <Mono className="min-w-0 flex-1 truncate text-sm">{revealedKey}</Mono>
            <IconButton aria-label={t("apiKeys.copyAria")} size="icon-sm" onClick={copyKey}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          </div>
          <p className="text-xs text-muted-foreground">
            {t("apiKeys.usageBefore")} <Mono className="text-xs">Authorization: Bearer {"<key>"}</Mono>{" "}
            {t("apiKeys.usageAfter")}
          </p>
          <DialogFooter>
            <Button onClick={() => setRevealedKey(null)}>{t("apiKeys.done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revokeTarget} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apiKeys.revokeTitle", { name: revokeTarget?.name })}</DialogTitle>
            <DialogDescription>{t("apiKeys.revokeDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRevokeTarget(null)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleRevoke}>
              {t("apiKeys.revoke")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
