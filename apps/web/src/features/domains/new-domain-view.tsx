"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateDomain } from "@/hooks/use-domains";
import { DomainForm, type DomainFormValues } from "@/features/domains/domain-form";

export function NewDomainView() {
  const router = useRouter();
  const canWrite = usePermission("settings.write");
  const createDomain = useCreateDomain();

  function handleSubmit(values: DomainFormValues) {
    createDomain.mutate(values, {
      onSuccess: (created) => {
        toast("Domain added", { description: values.domain });
        router.push(`/domains/${created.id}`);
      },
      onError: (err) => toast.error("Couldn't add domain", { description: err.message }),
    });
  }

  if (!canWrite) {
    return (
      <div className="flex flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">New Domain</h1>
        <PermissionDeniedState />
      </div>
    );
  }

  return <DomainForm title="New Domain" defaultValues={{}} submitLabel="Add domain" onSubmit={handleSubmit} />;
}
