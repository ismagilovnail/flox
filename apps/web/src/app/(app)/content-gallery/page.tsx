import { ContentGalleryView } from "@/features/content-gallery/content-gallery-view";
import { RequirePermission } from "@/components/shell/require-permission";

export default function Page() {
  return (
    <RequirePermission permission="content.read">
      <ContentGalleryView />
    </RequirePermission>
  );
}
