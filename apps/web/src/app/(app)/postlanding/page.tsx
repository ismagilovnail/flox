import { Suspense } from "react";

import { PostlandingList } from "@/features/postlanding/postlanding-list";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <Suspense>
        <PostlandingList />
      </Suspense>
    </RequirePermission>
  );
}
