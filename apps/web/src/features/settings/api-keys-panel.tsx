"use client";

import * as React from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { formatDistanceToNow } from "date-fns";
import { CopyIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { z } from "zod";

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
import { useSettingsStore } from "@/stores/settings";
import { API_KEY_SCOPES, type ApiKeyScope } from "@/lib/mock/settings";

const createKeySchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(60),
  scope: z.enum(API_KEY_SCOPES as [ApiKeyScope, ...ApiKeyScope[]]),
});

type CreateKeyValues = z.infer<typeof createKeySchema>;

const SCOPE_VARIANT: Record<ApiKeyScope, "outline" | "secondary" | "warning"> = {
  read: "outline",
  write: "secondary",
  admin: "warning",
};

export function ApiKeysPanel() {
  const apiKeys = useSettingsStore((s) => s.apiKeys);
  const createApiKey = useSettingsStore((s) => s.createApiKey);
  const revokeApiKey = useSettingsStore((s) => s.revokeApiKey);

  const [creating, setCreating] = React.useState(false);
  const [revealedKey, setRevealedKey] = React.useState<string | null>(null);
  const [revokeTarget, setRevokeTarget] = React.useState<{ id: string; name: string } | null>(null);

  const form = useForm<CreateKeyValues>({
    resolver: zodResolver(createKeySchema),
    defaultValues: { name: "", scope: "read" },
  });

  function handleCreate(values: CreateKeyValues) {
    const { fullKey } = createApiKey(values.name, values.scope);
    setCreating(false);
    form.reset();
    setRevealedKey(fullKey);
  }

  function copyKey() {
    if (!revealedKey) return;
    navigator.clipboard.writeText(revealedKey);
    toast("API key copied");
  }

  function handleRevoke() {
    if (!revokeTarget) return;
    revokeApiKey(revokeTarget.id);
    toast("API key revoked", { description: revokeTarget.name });
    setRevokeTarget(null);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>API keys</CardTitle>
        <CardDescription>Used for programmatic access. The full key is shown once, at creation.</CardDescription>
        <CardAction>
          <Button size="sm" onClick={() => setCreating(true)}>
            <PlusIcon className="size-4" /> New API key
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {apiKeys.length === 0 && <p className="px-4 py-6 text-sm text-muted-foreground">No API keys yet.</p>}
        {apiKeys.map((key) => (
          <div key={key.id} className="flex items-center gap-3 px-4 py-3">
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium">{key.name}</span>
              <Mono className="text-xs text-muted-foreground">{key.prefix}</Mono>
            </div>
            <Badge variant={SCOPE_VARIANT[key.scope]}>{key.scope}</Badge>
            <Caption className="hidden w-32 sm:block">
              {key.lastUsedAt ? `Used ${formatDistanceToNow(new Date(key.lastUsedAt), { addSuffix: true })}` : "Never used"}
            </Caption>
            <IconButton
              aria-label={`Revoke ${key.name}`}
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
            <DialogTitle>New API key</DialogTitle>
            <DialogDescription>Scope controls what this key can read or change.</DialogDescription>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(handleCreate)} className="flex flex-col gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="key-name">Name</Label>
              <Input id="key-name" placeholder="Reporting export" {...form.register("name")} />
              {form.formState.errors.name && (
                <p className="text-xs text-danger">{form.formState.errors.name.message}</p>
              )}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="key-scope">Scope</Label>
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
                          {s}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreating(false)}>
                Cancel
              </Button>
              <Button type="submit">Create key</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revealedKey} onOpenChange={(open) => !open && setRevealedKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Copy your API key now</DialogTitle>
            <DialogDescription>
              This is the only time it&apos;s shown in full. If you lose it, revoke this key and create a new one.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <Mono className="min-w-0 flex-1 truncate text-sm">{revealedKey}</Mono>
            <IconButton aria-label="Copy API key" size="icon-sm" onClick={copyKey}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          </div>
          <DialogFooter>
            <Button onClick={() => setRevealedKey(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!revokeTarget} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke &ldquo;{revokeTarget?.name}&rdquo;?</DialogTitle>
            <DialogDescription>Anything using this key stops working immediately.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRevokeTarget(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRevoke}>
              Revoke
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
