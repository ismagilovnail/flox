"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { LogOutIcon, SettingsIcon, UserIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLogout, useMe } from "@/hooks/use-auth";

function initials(name: string) {
  return name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export function UserMenu() {
  const { t } = useTranslation("auth");
  const me = useMe();
  const logout = useLogout();
  const router = useRouter();

  // AuthGuard (app/(app)/layout.tsx) already guarantees me.data exists by
  // the time anything under (app) renders — this null-check is only for
  // the brief window before that guard's own query resolves.
  if (!me.data) return null;
  const { user } = me.data;

  function handleLogout() {
    logout.mutate(undefined, { onSuccess: () => router.replace("/login") });
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="User menu"
          className="rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          <Avatar className="size-7">
            <AvatarFallback>{initials(user.name)}</AvatarFallback>
          </Avatar>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="flex flex-col gap-0.5 font-normal">
          <span className="text-sm font-medium text-foreground">{user.name}</span>
          <span className="text-xs text-muted-foreground">{user.email}</span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled>
          <UserIcon className="size-4" /> Profile
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link href="/settings">
            <SettingsIcon className="size-4" /> Settings
          </Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={handleLogout} disabled={logout.isPending}>
          <LogOutIcon className="size-4" /> {t("logout.menuItem")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
