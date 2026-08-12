import { Suspense } from "react";

import { PostlandingList } from "@/features/postlanding/postlanding-list";

export default function Page() {
  return (
    <Suspense>
      <PostlandingList />
    </Suspense>
  );
}
