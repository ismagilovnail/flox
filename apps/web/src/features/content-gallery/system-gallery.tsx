"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { EyeIcon, LayoutTemplateIcon, SearchIcon, SmartphoneIcon, ImageIcon, SparklesIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { usePermission } from "@/hooks/use-auth";
import { useApplySystemItem, useSystemGalleryItems, useSystemLandingPreview } from "@/hooks/use-content-gallery";
import { apiUrl } from "@/lib/api/client";
import type { SystemGalleryItem, SystemItemKind } from "@/lib/api/content-gallery";
import { usePwaTemplateDraft } from "@/stores/pwa-template-draft";

type KindFilter = "all" | SystemItemKind;
const KIND_ICON = { creative: ImageIcon, landing: LayoutTemplateIcon, pwa: SmartphoneIcon } as const;

/** The system catalog (§30.8): read-only content shipped with FLOX. "Use"
 * hands an item to its builder — a creative into the team library, a
 * landing template into a new landing (then its editor), a PWA template
 * into the new-PWA form — after which it is the team's own content. */
export function SystemGallery({ onOpenLibrary }: { onOpenLibrary: () => void }) {
  const { t, i18n } = useTranslation("contentGallery");
  const router = useRouter();
  const errorMessage = useApiErrorMessage();
  const canWrite = usePermission("content.write");
  const query = useSystemGalleryItems();
  const apply = useApplySystemItem();
  const [kind, setKind] = React.useState<KindFilter>("all");
  const [search, setSearch] = React.useState("");
  const [previewing, setPreviewing] = React.useState<SystemGalleryItem | null>(null);
  const [naming, setNaming] = React.useState<SystemGalleryItem | null>(null);
  const [landingName, setLandingName] = React.useState("");

  const lang = i18n.language?.startsWith("ru") ? "ru" : "en";
  const name = React.useCallback((it: SystemGalleryItem) => it.name[lang] ?? it.name.en, [lang]);
  const description = (it: SystemGalleryItem) => it.description[lang] ?? it.description.en;

  const items = React.useMemo(() => {
    const q = search.trim().toLowerCase();
    return (query.data?.items ?? []).filter(
      (it) =>
        (kind === "all" || it.kind === kind) &&
        (!q || [name(it), it.description[lang] ?? "", ...it.tags].some((s) => s.toLowerCase().includes(q))),
    );
  }, [query.data, kind, search, lang, name]);

  function applyItem(it: SystemGalleryItem, newName?: string) {
    apply.mutate(
      { id: it.id, name: newName },
      {
        onSuccess: (res) => {
          setPreviewing(null);
          setNaming(null);
          if (res.kind === "creative") {
            toast(t("system.toast.creative"), {
              description: name(it),
              action: { label: t("system.toast.openLibrary"), onClick: onOpenLibrary },
            });
          } else if (res.kind === "landing" && res.landing) {
            toast(t("system.toast.landing"), { description: res.landing.name });
            router.push(`/landings/${res.landing.id}`);
          } else if (res.kind === "pwa" && res.pwa) {
            usePwaTemplateDraft.getState().set({ templateName: name(it), fields: res.pwa });
            router.push("/pwa/new");
          }
        },
        onError: (err) => toast.error(t("system.toast.error"), { description: errorMessage(err) }),
      },
    );
  }

  // A landing gets a name first (it is created right away); the others go straight on.
  function startUse(it: SystemGalleryItem) {
    if (it.kind === "landing") {
      setPreviewing(null);
      setLandingName(name(it));
      setNaming(it);
      return;
    }
    applyItem(it);
  }

  if (query.isPending) return <LoadingState label={t("system.loading")} />;
  if (query.isError) {
    return <ErrorState title={t("system.loadError")} description={errorMessage(query.error)} onRetry={() => query.refetch()} />;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Tabs value={kind} onValueChange={(v) => setKind(v as KindFilter)}>
          <TabsList>
            <TabsTrigger value="all">{t("system.kinds.all")}</TabsTrigger>
            <TabsTrigger value="creative">{t("system.kinds.creative")}</TabsTrigger>
            <TabsTrigger value="landing">{t("system.kinds.landing")}</TabsTrigger>
            <TabsTrigger value="pwa">{t("system.kinds.pwa")}</TabsTrigger>
          </TabsList>
        </Tabs>
        <div className="relative w-full sm:w-64">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("system.search")}
            aria-label={t("system.search")}
            className="h-8 pl-8"
          />
        </div>
      </div>

      {items.length === 0 ? (
        <EmptyState icon={SparklesIcon} title={t("system.emptyTitle")} description={t("system.emptyDescription")} />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {items.map((it) => {
            const Icon = KIND_ICON[it.kind];
            return (
              <div key={it.id} className="flex flex-col overflow-hidden rounded-lg border border-border bg-card">
                <button
                  type="button"
                  onClick={() => setPreviewing(it)}
                  className="relative aspect-video overflow-hidden bg-muted/30"
                  aria-label={t("system.preview", { name: name(it) })}
                >
                  {/* eslint-disable-next-line @next/next/no-img-element -- served by the API, not a Next static asset */}
                  <img src={apiUrl(it.previewPath)} alt="" className="size-full object-contain" loading="lazy" />
                </button>
                <div className="flex flex-1 flex-col gap-2 p-3">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <Icon className="size-3.5" />
                    {t(`system.kindLabels.${it.kind}`)}
                  </div>
                  <p className="text-sm font-medium leading-tight">{name(it)}</p>
                  <p className="text-xs text-muted-foreground">{description(it)}</p>
                  <div className="flex flex-wrap gap-1">
                    {it.tags.map((tag) => (
                      <Badge key={tag} variant="secondary" className="font-normal">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                  <div className="mt-auto flex gap-2 pt-1">
                    <Button variant="outline" size="sm" onClick={() => setPreviewing(it)}>
                      <EyeIcon />
                      {t("system.previewAction")}
                    </Button>
                    {canWrite && (
                      <Button size="sm" disabled={apply.isPending} onClick={() => startUse(it)}>
                        {t(`system.use.${it.kind}`)}
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <PreviewDialog
        item={previewing}
        title={previewing ? name(previewing) : ""}
        canWrite={canWrite}
        pending={apply.isPending}
        onUse={startUse}
        onClose={() => setPreviewing(null)}
      />

      <Dialog open={naming !== null} onOpenChange={(open) => !open && setNaming(null)}>
        <DialogContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (naming && landingName.trim()) applyItem(naming, landingName.trim());
            }}
          >
            <DialogHeader>
              <DialogTitle>{t("system.landingDialog.title")}</DialogTitle>
              <DialogDescription>{t("system.landingDialog.description")}</DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="system-landing-name">{t("system.landingDialog.name")}</Label>
              <Input id="system-landing-name" value={landingName} onChange={(e) => setLandingName(e.target.value)} autoFocus />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setNaming(null)}>
                {t("system.landingDialog.cancel")}
              </Button>
              <Button type="submit" disabled={!landingName.trim() || apply.isPending}>
                {apply.isPending ? t("system.landingDialog.creating") : t("system.landingDialog.create")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function PreviewDialog({
  item,
  title,
  canWrite,
  pending,
  onUse,
  onClose,
}: {
  item: SystemGalleryItem | null;
  title: string;
  canWrite: boolean;
  pending: boolean;
  onUse: (it: SystemGalleryItem) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation("contentGallery");
  const errorMessage = useApiErrorMessage();
  const isLanding = item?.kind === "landing";
  const preview = useSystemLandingPreview(
    isLanding ? item.id : undefined,
    isLanding && item.assetBasePath ? apiUrl(item.assetBasePath) : undefined,
  );

  return (
    <Dialog open={item !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{item ? t(`system.previewHint.${item.kind}`) : ""}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-auto rounded-md border border-border bg-muted/30">
          {item && !isLanding && (
            // eslint-disable-next-line @next/next/no-img-element -- served by the API, not a Next static asset
            <img src={apiUrl(item.previewPath)} alt={title} className="mx-auto max-h-[60dvh] w-auto object-contain" />
          )}
          {isLanding && preview.isPending && <LoadingState label={t("system.previewLoading")} />}
          {isLanding && preview.isError && (
            <ErrorState title={t("system.previewError")} description={errorMessage(preview.error)} onRetry={() => preview.refetch()} />
          )}
          {isLanding && preview.isSuccess && (
            // Like the landing editor: scripts run, on an opaque origin with
            // no access to the app's cookies or storage; links go nowhere.
            <iframe
              title={title}
              srcDoc={preview.data.html}
              sandbox="allow-scripts"
              className="mx-auto h-[60dvh] w-full max-w-[420px] bg-white"
            />
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("system.close")}
          </Button>
          {item && canWrite && (
            <Button disabled={pending} onClick={() => onUse(item)}>
              {t(`system.use.${item.kind}`)}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
