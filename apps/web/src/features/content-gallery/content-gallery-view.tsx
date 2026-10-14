"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { GalleryBrowser } from "@/features/content-gallery/gallery-browser";
import { SystemGallery } from "@/features/content-gallery/system-gallery";

type Section = "library" | "system";

/** Two sections (§30.8 "distinguish system-provided gallery content from
 * team-uploaded content"): the team's own library, and the read-only
 * system catalog shipped with FLOX. */
export function ContentGalleryView() {
  const { t } = useTranslation("contentGallery");
  const [section, setSection] = React.useState<Section>("library");
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
        <Tabs value={section} onValueChange={(v) => setSection(v as Section)}>
          <TabsList>
            <TabsTrigger value="library">{t("sections.library")}</TabsTrigger>
            <TabsTrigger value="system">{t("sections.system")}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      {section === "library" ? <GalleryBrowser mode="manage" /> : <SystemGallery onOpenLibrary={() => setSection("library")} />}
    </div>
  );
}
