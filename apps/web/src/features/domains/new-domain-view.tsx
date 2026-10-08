"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateDomain } from "@/hooks/use-domains";
import { DomainForm, type DomainFormValues } from "@/features/domains/domain-form";

export function NewDomainView() {
  const { t } = useTranslation("domains");
  const router = useRouter();
  const canWrite = usePermission("settings.write");
  const createDomain = useCreateDomain();

  function handleSubmit(values: DomainFormValues) {
    createDomain.mutate(values, {
      onSuccess: (created) => {
        toast(t("form.added"), { description: values.domain });
        router.push(`/domains/${created.id}`);
      },
      onError: (err) => toast.error(t("form.addError"), { description: err.message }),
    });
  }

  if (!canWrite) {
    return (
      <div className="flex flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("form.newTitle")}</h1>
        <PermissionDeniedState />
      </div>
    );
  }

  return <DomainForm title={t("form.newTitle")} defaultValues={{}} submitLabel={t("form.add")} onSubmit={handleSubmit} />;
}
