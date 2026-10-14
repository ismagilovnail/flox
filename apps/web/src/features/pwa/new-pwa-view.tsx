"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreatePwa, useReplacePwaComments } from "@/hooks/use-pwas";
import { PwaForm, type PwaFormValues } from "@/features/pwa/pwa-form";
import { usePwaTemplateDraft, type PwaTemplateDraft } from "@/stores/pwa-template-draft";

/** A gallery template's fields as the form's values: screenshots are the
 * form's {url} rows; every other key already has the form's name. */
function templateValues(draft: PwaTemplateDraft): Partial<PwaFormValues> {
  const { screenshots, ...rest } = draft.fields;
  const urls = Array.isArray(screenshots) ? screenshots.filter((u): u is string => typeof u === "string") : [];
  return { ...rest, screenshots: urls.map((url) => ({ url })) } as Partial<PwaFormValues>;
}

export function NewPwaView() {
  const { t } = useTranslation("pwa");
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const createPwa = useCreatePwa();
  const replaceComments = useReplacePwaComments();
  // A template from the content gallery, read once: the form keeps it from
  // here on, and the store forgets it.
  const [template] = useState(() => usePwaTemplateDraft.getState().draft);
  useEffect(() => usePwaTemplateDraft.getState().clear(), []);

  function handleSubmit(values: PwaFormValues) {
    const { comments, ...pwaValues } = values;
    createPwa.mutate(
      {
        ...pwaValues,
        developer: values.developer ?? "",
        description: values.description ?? "",
        downloadsLabel: values.downloadsLabel ?? "",
        whatsNew: values.whatsNew ?? "",
        version: values.version ?? "",
        ageRating: values.ageRating ?? "",
        category: values.category ?? "",
        interactiveElements: values.interactiveElements ?? "",
        containsAds: values.containsAds,
        inAppPurchases: values.inAppPurchases,
        updatedDate: values.updatedDate || undefined,
        screenshots: values.screenshots.map((s) => s.url),
      },
      {
        onSuccess: async (created) => {
          toast(t("toast.created"), { description: values.name });
          if (comments.length > 0) {
            try {
              await replaceComments.mutateAsync({ id: created.id, comments });
            } catch (err) {
              toast.error(t("comments.saveError"), { description: err instanceof Error ? err.message : undefined });
            }
          }
          router.push(`/pwa/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  if (!canWrite) {
    return (
      <div className="flex flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("form.titleNew")}</h1>
        <PermissionDeniedState />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {template && (
        <Alert>
          <AlertDescription>{t("form.fromTemplate", { name: template.templateName })}</AlertDescription>
        </Alert>
      )}
      <PwaForm
        title={t("form.titleNew")}
        defaultValues={template ? templateValues(template) : {}}
        submitLabel={t("form.submitCreate")}
        onSubmit={handleSubmit}
      />
    </div>
  );
}
