"use client";

import { usePathname } from "next/navigation";
import { useTranslation } from "react-i18next";

import { findNavItem } from "@/lib/nav";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

function titleCase(segment: string) {
  return segment
    .split("-")
    .map((w) => w[0]?.toUpperCase() + w.slice(1))
    .join(" ");
}

export function ShellBreadcrumbs() {
  const pathname = usePathname();
  const { t } = useTranslation("nav");
  const segments = pathname.split("/").filter(Boolean);

  if (segments.length === 0) return null;

  // titleCase(segments[0]) is a fallback for a path with no matching nav
  // item (e.g. a detail page) — genuinely not a translatable string, since
  // it's derived from the URL segment itself, not product copy.
  const navItem = findNavItem(pathname);
  const label = navItem ? t(navItem.label) : titleCase(segments[0]);

  return (
    <Breadcrumb className="min-w-0">
      <BreadcrumbList className="flex-nowrap overflow-hidden">
        <BreadcrumbItem className="shrink-0">
          <BreadcrumbLink href="/overview">{t("brand")}</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator className="shrink-0" />
        <BreadcrumbItem className="min-w-0">
          <BreadcrumbPage className="block truncate">{label}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}
