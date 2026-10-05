"use client";

import { useState, type ReactNode } from "react";
import { CheckIcon, CornerDownRightIcon, ImagesIcon, StarIcon, ThumbsUpIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { GalleryFilePickerDialog } from "@/features/content-gallery/gallery-file-picker-dialog";
import type { PwaComment } from "@/lib/api/pwa";
import { cn } from "@/lib/utils";

const TEXT_MAX = 500;

export type CommentDraft = Pick<PwaComment, "author" | "avatarUrl" | "rating" | "likes" | "date" | "text" | "reply">;

export const EMPTY_COMMENT_DRAFT: CommentDraft = { author: "", avatarUrl: "", rating: 5, likes: 0, date: "", text: "", reply: "" };

/** The new/edit comment form. Everything is typed by the operator — the
 * date is either a specific day or left empty (then none is shown). The
 * parent keys this dialog per comment, so each open starts from `initial`. */
export function PwaCommentDialog({
  open,
  onOpenChange,
  setLabel,
  initial,
  editing,
  onSave,
  renderPreview,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  setLabel: string;
  initial: CommentDraft;
  editing: boolean;
  onSave: (draft: CommentDraft) => void;
  /** Live phone preview with this draft included. */
  renderPreview: (draft: CommentDraft) => ReactNode;
}) {
  const { t } = useTranslation("pwa");
  const [draft, setDraft] = useState<CommentDraft>(initial);
  const [pickingAvatar, setPickingAvatar] = useState(false);
  const patch = (p: Partial<CommentDraft>) => setDraft((d) => ({ ...d, ...p }));
  const valid = draft.author.trim() !== "" && draft.text.trim() !== "" && draft.rating >= 1 && draft.rating <= 5 && draft.likes >= 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-[1100px]">
        <DialogHeader>
          <DialogTitle>{editing ? t("comments.editTitle") : t("comments.newTitle")}</DialogTitle>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">
          {t("comments.willBeAdded")} <span className="rounded bg-muted px-1.5 py-0.5 tabular-nums">{setLabel}</span>
        </p>

        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_400px]">
          <div className="flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-[120px_1fr]">
          <div className="relative w-fit">
            <button
              type="button"
              aria-label={t("comments.avatar")}
              onClick={() => setPickingAvatar(true)}
              className="flex size-28 items-center justify-center overflow-hidden rounded-lg border-2 border-dashed text-muted-foreground transition-colors hover:border-muted-foreground/50 hover:text-foreground"
            >
              {draft.avatarUrl ? (
                // eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL, not a Next-optimizable static asset
                <img src={draft.avatarUrl} alt="" className="size-full object-cover" />
              ) : (
                <span className="flex flex-col items-center gap-1 px-2 text-center text-[11px] font-medium">
                  <ImagesIcon className="size-5" />
                  {t("comments.avatarPick")}
                </span>
              )}
            </button>
            {draft.avatarUrl && (
              <button
                type="button"
                aria-label={t("comments.avatarRemove")}
                onClick={() => patch({ avatarUrl: "" })}
                className="absolute -top-1.5 -right-1.5 rounded-full bg-secondary p-0.5 shadow"
              >
                <XIcon className="size-3.5" />
              </button>
            )}
            <GalleryFilePickerDialog open={pickingAvatar} onOpenChange={setPickingAvatar} requiredAspect={1} onPick={(f) => patch({ avatarUrl: f.url })} />
          </div>

          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="pwa-comment-author">{t("comments.author")}</Label>
              <Input id="pwa-comment-author" maxLength={100} placeholder={t("comments.authorPlaceholder")} value={draft.author} onChange={(e) => patch({ author: e.target.value })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label id="pwa-comment-rating-label">{t("comments.rating")}</Label>
                <div className="flex h-9 items-center gap-1" role="radiogroup" aria-labelledby="pwa-comment-rating-label">
                  {[1, 2, 3, 4, 5].map((n) => (
                    <button key={n} type="button" role="radio" aria-checked={draft.rating === n} aria-label={String(n)} onClick={() => patch({ rating: n })}>
                      <StarIcon className={cn("size-6", n <= draft.rating ? "fill-amber-400 text-amber-400" : "text-muted-foreground/40")} />
                    </button>
                  ))}
                </div>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pwa-comment-likes">{t("comments.likes")}</Label>
                <div className="relative">
                  <ThumbsUpIcon className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
                  <Input id="pwa-comment-likes" type="number" min={0} max={1000000} step={1} className="pl-9" value={draft.likes} onChange={(e) => patch({ likes: Math.max(0, Math.floor(Number(e.target.value) || 0)) })} />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor="pwa-comment-date">{t("comments.date")}</Label>
          <Input id="pwa-comment-date" type="date" className="w-48" value={draft.date} onChange={(e) => patch({ date: e.target.value })} />
          <p className="text-xs text-muted-foreground">{t("comments.dateHint")}</p>
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor="pwa-comment-text">{t("comments.text")}</Label>
          <Textarea id="pwa-comment-text" rows={3} maxLength={TEXT_MAX} placeholder={t("comments.textPlaceholder")} value={draft.text} onChange={(e) => patch({ text: e.target.value })} />
          <span className="text-right text-xs text-muted-foreground tabular-nums">{draft.text.length} / {TEXT_MAX}</span>
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor="pwa-comment-reply" className="flex items-center gap-1.5">
            <CornerDownRightIcon className="size-4" /> {t("comments.reply")}
          </Label>
          <Textarea id="pwa-comment-reply" rows={2} maxLength={TEXT_MAX} placeholder={t("comments.replyPlaceholder")} value={draft.reply} onChange={(e) => patch({ reply: e.target.value })} />
          <span className="text-right text-xs text-muted-foreground tabular-nums">{draft.reply.length} / {TEXT_MAX}</span>
        </div>

          </div>
          <div className="hidden lg:block">
            <div className="sticky top-0">{renderPreview(draft)}</div>
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("comments.cancel")}
          </Button>
          <Button type="button" disabled={!valid} onClick={() => onSave({ ...draft, author: draft.author.trim(), text: draft.text.trim(), reply: draft.reply.trim() })}>
            <CheckIcon className="size-4" /> {t("comments.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
