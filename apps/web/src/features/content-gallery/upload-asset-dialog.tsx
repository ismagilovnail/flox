"use client";

import * as React from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useContentGalleryStore } from "@/stores/content-gallery";

const schema = z.object({
  title: z.string().min(2, "Name must be at least 2 characters").max(80),
  description: z.string().min(2, "Add a short description").max(300),
  fileType: z.enum(["image", "video", "zip"]),
  fileUrl: z.url("Enter a valid, already-hosted URL"),
  tags: z.string().max(100),
});

type FormValues = z.infer<typeof schema>;

export function UploadAssetDialog({
  open,
  onOpenChange,
  memberId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  memberId: string;
}) {
  const uploadTeamAsset = useContentGalleryStore((s) => s.uploadTeamAsset);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { title: "", description: "", fileType: "image", fileUrl: "", tags: "" },
  });
  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  function onSubmit(values: FormValues) {
    uploadTeamAsset({
      title: values.title,
      description: values.description,
      fileType: values.fileType,
      fileUrl: values.fileUrl,
      tags: values.tags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
      memberId,
    });
    toast("Asset added to gallery", { description: values.title });
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload a team asset</DialogTitle>
          <DialogDescription>
            Team uploads are private to your workspace — other tenants never see them (§36-TENANCY). Point at a URL
            you already host; there&apos;s no object-storage upload in this build yet.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="asset-title">Name</Label>
            <Input id="asset-title" placeholder="Q4 Promo Banner" {...register("title")} aria-invalid={!!errors.title} />
            {errors.title && <p className="text-xs text-danger">{errors.title.message}</p>}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="asset-desc">Description</Label>
            <Textarea id="asset-desc" {...register("description")} aria-invalid={!!errors.description} />
            {errors.description && <p className="text-xs text-danger">{errors.description.message}</p>}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="asset-type">File type</Label>
              <Controller
                control={control}
                name="fileType"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="asset-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="image">image</SelectItem>
                      <SelectItem value="video">video</SelectItem>
                      <SelectItem value="zip">zip</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="asset-tags">Tags (comma-separated)</Label>
              <Input id="asset-tags" placeholder="banner, q4" {...register("tags")} />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="asset-url">Hosted URL</Label>
            <Input
              id="asset-url"
              className="font-mono text-xs"
              placeholder="https://assets.example-team.com/banner.png"
              {...register("fileUrl")}
              aria-invalid={!!errors.fileUrl}
            />
            {errors.fileUrl && <p className="text-xs text-danger">{errors.fileUrl.message}</p>}
          </div>
          <DialogFooter className="mt-0 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              Add to gallery
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
