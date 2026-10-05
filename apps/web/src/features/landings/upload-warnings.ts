import { toast } from "sonner";
import type { TFunction } from "i18next";

/** Surfaces an archive upload's root-absolute asset references — the
 * landing was saved, but these paths won't load from the bundle. */
export function toastUploadWarnings(t: TFunction, warnings: string[]) {
  if (warnings.length === 0) return;
  toast.warning(t("toast.absolutePathsTitle", { ns: "landings", count: warnings.length }), {
    description: `${t("toast.absolutePathsDescription", { ns: "landings" })} ${warnings.join(", ")}`,
    duration: 12000,
  });
}
