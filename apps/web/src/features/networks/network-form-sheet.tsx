"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { MacroPicker } from "@/components/shared/macro-picker";
import type { NetworkStatus } from "@/lib/mock/networks";

export const networkFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  postbackUrl: z.url("Enter a valid URL"),
  status: z.enum(["active", "paused", "archived"] as [NetworkStatus, ...NetworkStatus[]]),
});

export type NetworkFormValues = z.infer<typeof networkFormSchema>;

const STATUS_OPTIONS: NetworkStatus[] = ["active", "paused", "archived"];

export function NetworkFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<NetworkFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: NetworkFormValues) => void;
}) {
  const form = useForm<NetworkFormValues>({
    resolver: zodResolver(networkFormSchema),
    defaultValues: { name: "", postbackUrl: "", status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>Networks are the CPA/CPL partners your offers belong to (§27).</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="net-name">Name</Label>
            <Input id="net-name" placeholder="AffTrust CPA" {...register("name")} aria-invalid={!!errors.name} />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="net-postback">Postback URL</Label>
              <MacroPicker onInsert={(token) => setValue("postbackUrl", `${getValues("postbackUrl")}${token}`)} />
            </div>
            <Input
              id="net-postback"
              placeholder="https://network.example/postback?click_id={click_id}&status={status}"
              className="font-mono text-xs"
              {...register("postbackUrl")}
              aria-invalid={!!errors.postbackUrl}
            />
            {errors.postbackUrl && <p className="text-xs text-danger">{errors.postbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">
              Fired when a conversion status changes for this network (Phase 13 wires the real delivery).
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="net-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="net-status" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {STATUS_OPTIONS.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {submitLabel}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
