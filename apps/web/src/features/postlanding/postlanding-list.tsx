"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { usePostlandingsStore } from "@/stores/postlandings";
import { postlandingColumns } from "@/features/postlanding/postlanding-columns";
import { PostlandingFormSheet, type PostlandingFormValues } from "@/features/postlanding/postlanding-form-sheet";
import type { Postlanding } from "@/lib/mock/postlandings";

export function PostlandingList() {
  const postlandings = usePostlandingsStore((s) => s.postlandings);
  const addPostlanding = usePostlandingsStore((s) => s.addPostlanding);
  const updatePostlanding = usePostlandingsStore((s) => s.updatePostlanding);

  const [target, setTarget] = React.useState<Postlanding | null | undefined>(undefined);

  function handleSubmit(values: PostlandingFormValues) {
    if (target) {
      updatePostlanding(target.id, values);
      toast("Postlanding updated", { description: values.name });
    } else {
      addPostlanding(values);
      toast("Postlanding created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => postlandingColumns((postlanding) => setTarget(postlanding)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Postlanding</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Postlanding
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={postlandings}
        searchPlaceholder="Search postlandings..."
        emptyTitle="No postlandings yet"
        emptyDescription="Add a postlanding page to use after the offer/PWA step of a flow."
        pageSize={10}
      />

      {target !== undefined && (
        <PostlandingFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Postlanding"}
          submitLabel={target ? "Save changes" : "Create postlanding"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
