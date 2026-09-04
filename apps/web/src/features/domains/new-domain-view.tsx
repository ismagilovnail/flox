"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useCreateDomain } from "@/hooks/use-domains";
import { DomainForm, type DomainFormValues } from "@/features/domains/domain-form";

export function NewDomainView() {
  const router = useRouter();
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

  return <DomainForm title="New Domain" defaultValues={{}} submitLabel="Add domain" onSubmit={handleSubmit} />;
}
