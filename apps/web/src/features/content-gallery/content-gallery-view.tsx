"use client";

import { useTranslation } from "react-i18next";

import { GalleryBrowser } from "@/features/content-gallery/gallery-browser";

export function ContentGalleryView() {
  const { t } = useTranslation("contentGallery");
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
      </div>
      <GalleryBrowser mode="manage" />
    </div>
  );
}
