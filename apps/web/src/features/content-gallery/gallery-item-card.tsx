import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { GalleryPreviewTile } from "@/features/content-gallery/gallery-preview-tile";
import { GALLERY_CATEGORY_LABELS, type GalleryItem } from "@/lib/mock/content-gallery";

export function GalleryItemCard({ item, onClick }: { item: GalleryItem; onClick: () => void }) {
  return (
    <Card
      size="sm"
      className="cursor-pointer gap-3 p-0 transition-shadow hover:shadow-md"
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onClick()}
    >
      <GalleryPreviewTile item={item} />
      <CardHeader className="px-3">
        <CardTitle className="line-clamp-1">{item.title}</CardTitle>
      </CardHeader>
      <CardContent className="px-3 text-xs text-muted-foreground">
        <p className="line-clamp-2">{item.description}</p>
      </CardContent>
      <CardFooter className="justify-between border-t-0 bg-transparent px-3 pb-3">
        <Badge variant="outline">{GALLERY_CATEGORY_LABELS[item.category]}</Badge>
        {item.source === "team" && <Badge variant="info">Team</Badge>}
      </CardFooter>
    </Card>
  );
}
