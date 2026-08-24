"use client";

import { PanelLeftIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar";
import { NavContent } from "@/components/shell/nav-content";
import { IconButton } from "@/components/ui/icon-button";

export function Sidebar() {
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toggle = useSidebarStore((s) => s.toggle);
  const { t } = useTranslation("nav");

  return (
    <aside
      className={cn(
        "sticky top-0 hidden h-dvh shrink-0 flex-col border-r border-border bg-sidebar transition-[width] duration-150 md:flex",
        collapsed ? "w-14" : "w-60",
      )}
    >
      <div className="flex-1 overflow-y-auto">
        <NavContent collapsed={collapsed} />
      </div>
      <div className="flex items-center justify-end border-t border-border p-2">
        <IconButton
          aria-label={t(collapsed ? "sidebar.expandAria" : "sidebar.collapseAria")}
          variant="ghost"
          size="icon-sm"
          onClick={toggle}
        >
          <PanelLeftIcon className="size-4" />
        </IconButton>
      </div>
    </aside>
  );
}
