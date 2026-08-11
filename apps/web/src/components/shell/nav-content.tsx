"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { NAV_GROUPS } from "@/lib/nav";
import { WorkspaceSelector } from "@/components/shell/workspace-selector";

export function NavContent({
  collapsed = false,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const pathname = usePathname();

  return (
    <div className="flex h-full flex-col gap-1 p-2">
      <div className="mb-1">
        <WorkspaceSelector collapsed={collapsed} />
      </div>
      <nav className="flex flex-1 flex-col gap-4 overflow-y-auto">
        {NAV_GROUPS.map((group, i) => (
          <div key={group.label ?? i} className="flex flex-col gap-0.5">
            {group.label && !collapsed && (
              <span className="px-2 py-1 text-[0.6875rem] font-medium uppercase tracking-wide text-muted-foreground">
                {group.label}
              </span>
            )}
            {group.items.map((item) => {
              const active =
                pathname === item.href || pathname.startsWith(`${item.href}/`);
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onNavigate}
                  aria-current={active ? "page" : undefined}
                  title={collapsed ? item.label : undefined}
                  className={cn(
                    "flex h-8 items-center gap-2.5 rounded-md px-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                    active && "bg-accent text-accent-foreground",
                    collapsed && "justify-center px-0",
                  )}
                >
                  <Icon className="size-4 shrink-0" />
                  {!collapsed && <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
    </div>
  );
}
