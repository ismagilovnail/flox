"use client";

import * as React from "react";
import { CheckIcon, FolderIcon, FolderPlusIcon, PencilIcon, Trash2Icon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { usePermission } from "@/hooks/use-auth";
import { cn } from "@/lib/utils";
import {
  useCreateGalleryFolder,
  useDeleteGalleryFolder,
  useGalleryFolders,
  useRenameGalleryFolder,
} from "@/hooks/use-content-gallery";
import type { GalleryFolder } from "@/lib/api/content-gallery";

/** "root" is this component's own sentinel for "unfiled only" — the
 * reference's own folder panel toggles between "every file" and one
 * specific folder (root included as one of the choices), matching
 * apps/internal/contentgallery's ListFilesParams.FolderID tri-state. */
export const GALLERY_ROOT = "root";

export function GalleryFolderPanel({
  selected,
  onSelect,
  className,
}: {
  selected: string | undefined;
  onSelect: (value: string | undefined) => void;
  className?: string;
}) {
  const canWrite = usePermission("content.write");
  const { data } = useGalleryFolders();
  const folders = data?.folders ?? [];

  const createFolder = useCreateGalleryFolder();
  const [creating, setCreating] = React.useState(false);
  const [newName, setNewName] = React.useState("");

  function submitCreate() {
    const name = newName.trim();
    if (!name) return;
    createFolder.mutate(name, {
      onSuccess: () => {
        setNewName("");
        setCreating(false);
      },
      onError: (err) => toast.error("Couldn't create folder", { description: err.message }),
    });
  }

  return (
    <div className={cn("flex w-56 shrink-0 flex-col gap-1", className)}>
      <FolderRow
        icon={FolderIcon}
        label="All files"
        active={selected === undefined}
        onClick={() => onSelect(undefined)}
      />
      <FolderRow
        icon={FolderIcon}
        label="Unfiled"
        active={selected === GALLERY_ROOT}
        onClick={() => onSelect(GALLERY_ROOT)}
      />

      <div className="my-1 h-px bg-border" />

      {folders.map((folder) => (
        <FolderItem
          key={folder.id}
          folder={folder}
          active={selected === folder.id}
          onSelect={() => onSelect(folder.id)}
          canWrite={canWrite}
        />
      ))}

      {canWrite &&
        (creating ? (
          <div className="flex items-center gap-1 px-1">
            <Input
              autoFocus
              value={newName}
              maxLength={50}
              placeholder="Folder name"
              className="h-8 text-sm"
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") submitCreate();
                if (e.key === "Escape") setCreating(false);
              }}
            />
            <IconButton size="icon-sm" variant="ghost" onClick={submitCreate} disabled={createFolder.isPending} aria-label="Save folder">
              <CheckIcon className="size-3.5" />
            </IconButton>
            <IconButton size="icon-sm" variant="ghost" onClick={() => setCreating(false)} aria-label="Cancel">
              <XIcon className="size-3.5" />
            </IconButton>
          </div>
        ) : (
          <Button variant="ghost" size="sm" className="justify-start gap-2 text-muted-foreground" onClick={() => setCreating(true)}>
            <FolderPlusIcon className="size-3.5" /> New folder
          </Button>
        ))}
    </div>
  );
}

function FolderRow({
  icon: Icon,
  label,
  active,
  onClick,
}: {
  icon: typeof FolderIcon;
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors",
        active ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
      )}
    >
      <Icon className="size-3.5 shrink-0" />
      <span className="truncate">{label}</span>
    </button>
  );
}

function FolderItem({
  folder,
  active,
  onSelect,
  canWrite,
}: {
  folder: GalleryFolder;
  active: boolean;
  onSelect: () => void;
  canWrite: boolean;
}) {
  const renameFolder = useRenameGalleryFolder();
  const deleteFolder = useDeleteGalleryFolder();
  const [editing, setEditing] = React.useState(false);
  const [name, setName] = React.useState(folder.name);
  const [confirmDelete, setConfirmDelete] = React.useState(false);

  function submitRename() {
    const trimmed = name.trim();
    if (!trimmed || trimmed === folder.name) {
      setEditing(false);
      setName(folder.name);
      return;
    }
    renameFolder.mutate(
      { id: folder.id, name: trimmed },
      {
        onSuccess: () => setEditing(false),
        onError: (err) => toast.error("Couldn't rename folder", { description: err.message }),
      },
    );
  }

  function submitDelete() {
    deleteFolder.mutate(folder.id, {
      onSuccess: () => {
        toast("Folder deleted", { description: "Its files were moved to Unfiled, not deleted." });
        setConfirmDelete(false);
      },
      onError: (err) => toast.error("Couldn't delete folder", { description: err.message }),
    });
  }

  if (editing) {
    return (
      <div className="flex items-center gap-1 px-1">
        <Input
          autoFocus
          value={name}
          maxLength={50}
          className="h-8 text-sm"
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitRename();
            if (e.key === "Escape") {
              setEditing(false);
              setName(folder.name);
            }
          }}
        />
        <IconButton size="icon-sm" variant="ghost" onClick={submitRename} disabled={renameFolder.isPending} aria-label="Save name">
          <CheckIcon className="size-3.5" />
        </IconButton>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "group flex items-center gap-1 rounded-md pr-1 pl-0.5 text-sm transition-colors",
        active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
      )}
    >
      <button type="button" onClick={onSelect} className={cn("flex min-w-0 flex-1 items-center gap-2 py-1.5 pl-2 text-left", active && "font-medium")}>
        <FolderIcon className="size-3.5 shrink-0" />
        <span className="truncate">{folder.name}</span>
      </button>
      {canWrite && (
        <IconButton
          size="icon-sm"
          variant="ghost"
          className="opacity-0 group-hover:opacity-100"
          onClick={() => setEditing(true)}
          aria-label={`Rename ${folder.name}`}
        >
          <PencilIcon className="size-3" />
        </IconButton>
      )}
      {canWrite && (
        <IconButton
          size="icon-sm"
          variant="ghost"
          className="opacity-0 group-hover:opacity-100 hover:text-danger"
          onClick={() => setConfirmDelete(true)}
          aria-label={`Delete ${folder.name}`}
        >
          <Trash2Icon className="size-3" />
        </IconButton>
      )}

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete &ldquo;{folder.name}&rdquo;?</DialogTitle>
            <DialogDescription>Its files move to Unfiled — they are not deleted.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={submitDelete} disabled={deleteFolder.isPending}>
              Delete folder
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
