"use client";

import * as React from "react";
import { CheckIcon, ChevronsUpDownIcon, PlusIcon } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const WORKSPACES = [
  { id: "org_1", name: "Nail Ismagilov" },
  { id: "org_2", name: "Blue Ridge Media" },
  { id: "org_3", name: "Apex Traffic" },
];

export function WorkspaceSelector({ collapsed }: { collapsed?: boolean }) {
  const [activeId, setActiveId] = React.useState(WORKSPACES[0].id);
  const active = WORKSPACES.find((w) => w.id === activeId) ?? WORKSPACES[0];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          className={cn(
            "h-9 w-full justify-start gap-2 px-2",
            collapsed && "justify-center px-0",
          )}
        >
          <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary text-[0.6875rem] font-semibold text-primary-foreground">
            {active.name.slice(0, 1)}
          </span>
          {!collapsed && (
            <>
              <span className="flex-1 truncate text-left text-sm font-medium">
                {active.name}
              </span>
              <ChevronsUpDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
            </>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {WORKSPACES.map((ws) => (
          <DropdownMenuItem key={ws.id} onSelect={() => setActiveId(ws.id)}>
            <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-secondary text-[0.6875rem] font-semibold">
              {ws.name.slice(0, 1)}
            </span>
            {ws.name}
            {ws.id === activeId && (
              <CheckIcon className="ml-auto size-3.5 text-primary" />
            )}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem>
          <PlusIcon className="size-4" /> New workspace
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
