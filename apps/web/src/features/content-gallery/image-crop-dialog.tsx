"use client";

import { useTranslation } from "react-i18next";

import * as React from "react";
import ReactCrop, { centerCrop, cropToCanvas, makeAspectCrop, type Crop, type PixelCrop } from "react-image-crop";
import "react-image-crop/dist/ReactCrop.css";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/** react-image-crop can't crop an animated GIF (canvas has no concept of
 * frames — it would silently flatten to one static frame), so a GIF
 * source skips this dialog entirely rather than quietly destroying the
 * animation. See gallery-browser.tsx's own call site. */
export const CROPPABLE_MIME_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);

const MIME_TO_EXT: Record<string, string> = {
  "image/jpeg": ".jpg",
  "image/png": ".png",
  "image/webp": ".webp",
};

function centeredAspectCrop(width: number, height: number, aspect: number): Crop {
  return centerCrop(makeAspectCrop({ unit: "%", width: 90 }, aspect, width, height), width, height);
}

/** The reference's own "attach an icon" flow: pick or upload a file
 * as-is (Content Gallery uploads are never cropped — see
 * gallery-upload-cell.tsx), then crop happens once, right when that file
 * is actually attached to a field that requires a specific aspect ratio
 * — e.g. a PWA icon, which is conventionally square. The ratio is fixed
 * by the caller (gallery-browser.tsx's `requiredAspect`), not chosen
 * here. The image itself never pans or zooms — the user draws/resizes a
 * fixed-aspect selection rectangle over a static image, matching the
 * reference's own interaction exactly (this replaced an earlier pan-and-
 * zoom-behind-a-fixed-frame version built on a different library, after
 * the user pointed out the reference works the other way around). */
export function ImageCropDialog({
  imageUrl,
  filename,
  mimeType,
  aspect,
  open,
  onOpenChange,
  onConfirm,
  onSkip,
}: {
  imageUrl: string | null;
  filename: string;
  mimeType: string;
  aspect: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (cropped: File) => void;
  onSkip: () => void;
}) {
  const { t } = useTranslation("contentGallery");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("crop.title")}</DialogTitle>
          <DialogDescription>{t("crop.description", { aspect: aspect.toFixed(2) })}</DialogDescription>
        </DialogHeader>
        {/* Keyed by the source itself — a fresh image means a fresh
         * centered selection, via a full remount rather than an effect
         * that resets state on every prop change (the React-recommended
         * alternative: https://react.dev/learn/you-might-not-need-an-effect
         * — "resetting state when a prop changes"). */}
        {imageUrl && (
          <CropperBody
            key={imageUrl}
            imageUrl={imageUrl}
            filename={filename}
            mimeType={mimeType}
            aspect={aspect}
            onConfirm={onConfirm}
            onSkip={onSkip}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function CropperBody({
  imageUrl,
  filename,
  mimeType,
  aspect,
  onConfirm,
  onSkip,
}: {
  imageUrl: string;
  filename: string;
  mimeType: string;
  aspect: number;
  onConfirm: (cropped: File) => void;
  onSkip: () => void;
}) {
  const { t } = useTranslation("contentGallery");
  const imgRef = React.useRef<HTMLImageElement>(null);
  const [crop, setCrop] = React.useState<Crop>();
  const [completedCrop, setCompletedCrop] = React.useState<PixelCrop>();
  const [applying, setApplying] = React.useState(false);

  function onImageLoad(e: React.SyntheticEvent<HTMLImageElement>) {
    const { width, height } = e.currentTarget;
    setCrop(centeredAspectCrop(width, height, aspect));
  }

  async function apply() {
    const image = imgRef.current;
    if (!image || !completedCrop || completedCrop.width === 0 || completedCrop.height === 0) return;
    setApplying(true);
    try {
      const canvas = document.createElement("canvas");
      await cropToCanvas(image, canvas, completedCrop);

      const requestedType = CROPPABLE_MIME_TYPES.has(mimeType) ? mimeType : "image/png";
      const blob = await new Promise<Blob>((resolve, reject) => {
        // The output filename's extension is derived from the produced
        // Blob's own `type`, not the *requested* one — canvas.toBlob
        // silently falls back to PNG if a browser can't encode the
        // requested type, and apps/internal/contentgallery's upload
        // validation cross-checks extension against sniffed content, so
        // a mismatched extension would get a correctly-cropped image
        // rejected for no visible reason.
        canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("Crop failed"))), requestedType, 0.92);
      });

      const ext = MIME_TO_EXT[blob.type] ?? ".png";
      const baseName = filename.replace(/\.[^./\\]+$/, "");
      onConfirm(new File([blob], `${baseName}${ext}`, { type: blob.type }));
    } catch {
      // Falls back to the untouched original rather than leaving the
      // user stuck on a broken dialog — most likely cause is a canvas
      // tainted by a CORS-less response; the gallery's own storage setup
      // should prevent that (apps/internal/storage's own doc has the
      // full story), but a user is never blocked on it either way.
      onSkip();
    } finally {
      setApplying(false);
    }
  }

  return (
    <>
      <div className="flex max-h-[60vh] justify-center overflow-auto rounded-md bg-muted p-2">
        <ReactCrop crop={crop} onChange={(_pixelCrop, percentCrop) => setCrop(percentCrop)} onComplete={setCompletedCrop} aspect={aspect} keepSelection>
          {/* eslint-disable-next-line @next/next/no-img-element -- react-image-crop needs a real <img> ref (imgRef), not next/image's wrapper */}
          <img
            ref={imgRef}
            src={imageUrl}
            alt=""
            crossOrigin="anonymous"
            onLoad={onImageLoad}
            className="max-h-[56vh] w-auto"
          />
        </ReactCrop>
      </div>

      <DialogFooter className="mt-0 flex-row items-center justify-between">
        <Button type="button" variant="ghost" onClick={onSkip}>
          {t("crop.skip")}
        </Button>
        <Button type="button" onClick={apply} disabled={!completedCrop || applying}>
          {applying ? t("crop.applying") : t("crop.apply")}
        </Button>
      </DialogFooter>
    </>
  );
}
