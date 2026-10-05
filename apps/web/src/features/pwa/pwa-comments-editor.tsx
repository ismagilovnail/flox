"use client";

import { useRef, useState } from "react";
import { DownloadIcon, PlusIcon, StarIcon, Trash2Icon, UploadIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ChevronDownIcon } from "lucide-react";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { PwaComment } from "@/lib/api/pwa";
import { COMMENT_GEOS, commentsToCsv, displayName, parseCommentsCsv, setKey } from "@/features/pwa/pwa-comments";
import { cn } from "@/lib/utils";

const MAX_COMMENTS = 500;
const EMPTY_DRAFT = { author: "", rating: 5, text: "" };

/** Hand-entered comments for the install page, grouped into sets by geo +
 * language. Everything here is typed or imported by the operator — there is
 * deliberately no generator. The list lives in the PWA form's state and is
 * saved together with it. */
export function PwaCommentsEditor({
  value,
  onChange,
  activeSet,
  onActiveSetChange,
}: {
  value: PwaComment[];
  onChange: (next: PwaComment[]) => void;
  activeSet: string;
  onActiveSetChange: (key: string) => void;
}) {
  const { t, i18n } = useTranslation("pwa");
  const locale = i18n.language;
  const fileRef = useRef<HTMLInputElement>(null);
  const [selected, setSelected] = useState<Set<PwaComment>>(new Set());
  const [newSet, setNewSet] = useState({ geo: "", lang: "" });
  const [extraSets, setExtraSets] = useState<string[]>([]);
  const [draft, setDraft] = useState(EMPTY_DRAFT);
  const [adding, setAdding] = useState(false);
  const [geoOpen, setGeoOpen] = useState(false);

  const sets = Array.from(new Set([...value.map(setKey), ...extraSets]));
  const inSet = value.filter((c) => setKey(c) === activeSet);
  const countOf = (key: string) => value.filter((c) => setKey(c) === key).length;
  const allSelected = inSet.length > 0 && inSet.every((c) => selected.has(c));

  const setValid = newSet.geo !== "" && newSet.lang !== "";
  const sortedGeos = [...COMMENT_GEOS].sort((a, b) =>
    displayName("region", a.geo, locale).localeCompare(displayName("region", b.geo, locale), locale),
  );
  const geoLangs = COMMENT_GEOS.find((g) => g.geo === newSet.geo)?.langs ?? [];
  const draftValid = draft.author.trim() !== "" && draft.text.trim() !== "" && draft.rating >= 1 && draft.rating <= 5;

  function addSet() {
    if (!setValid) return;
    const key = `${newSet.geo}-${newSet.lang}`;
    if (!sets.includes(key)) setExtraSets((prev) => [...prev, key]);
    onActiveSetChange(key);
    setNewSet({ geo: "", lang: "" });
  }

  function removeSet(key: string) {
    onChange(value.filter((c) => setKey(c) !== key));
    setExtraSets((prev) => prev.filter((k) => k !== key));
    setSelected(new Set());
    if (activeSet === key) onActiveSetChange(sets.find((k) => k !== key) ?? "");
  }

  function addComment() {
    if (!activeSet || !draftValid || value.length >= MAX_COMMENTS) return;
    const [geo, lang] = activeSet.split("-");
    onChange([...value, { geo, lang, author: draft.author.trim(), rating: draft.rating, text: draft.text.trim() }]);
    setDraft(EMPTY_DRAFT);
    setAdding(false);
  }

  function deleteSelected() {
    onChange(value.filter((c) => !selected.has(c)));
    setSelected(new Set());
  }

  function toggle(c: PwaComment) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(c)) next.delete(c);
      else next.add(c);
      return next;
    });
  }

  async function importCsv(file: File) {
    const { comments, skipped } = parseCommentsCsv(await file.text());
    const room = MAX_COMMENTS - value.length;
    const accepted = comments.slice(0, room);
    if (accepted.length > 0) {
      onChange([...value, ...accepted]);
      if (!activeSet) onActiveSetChange(setKey(accepted[0]));
    }
    const dropped = skipped + (comments.length - accepted.length);
    if (accepted.length === 0) {
      toast.error(t("comments.importNone"));
    } else {
      toast(t("comments.imported", { count: accepted.length }), {
        description: dropped > 0 ? t("comments.importSkipped", { count: dropped }) : undefined,
      });
    }
  }

  function exportCsv() {
    const url = URL.createObjectURL(new Blob([commentsToCsv(value)], { type: "text/csv;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "pwa-comments.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("comments.title")}</CardTitle>
        <CardDescription>{t("comments.hint")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label>{t("comments.setsLabel")}</Label>
          <div className="flex flex-wrap gap-2">
            {sets.map((key) => (
              <span
                key={key}
                className={cn(
                  "inline-flex items-center gap-1 rounded-md border pl-2 text-sm",
                  key === activeSet ? "border-primary bg-primary/10" : "border-border",
                )}
              >
                <button type="button" className="py-1 tabular-nums" onClick={() => { onActiveSetChange(key); setSelected(new Set()); }}>
                  {key.replace("-", " · ")} <span className="text-muted-foreground">({countOf(key)})</span>
                </button>
                <IconButton type="button" aria-label={t("comments.removeSetAria", { set: key })} onClick={() => removeSet(key)}>
                  <XIcon className="size-3.5" />
                </IconButton>
              </span>
            ))}
          </div>
          <div className="flex flex-wrap items-end gap-2">
            <div className="grid gap-1">
              <Label htmlFor="pwa-set-geo" className="text-xs">{t("comments.geo")}</Label>
              <Popover open={geoOpen} onOpenChange={setGeoOpen}>
                <PopoverTrigger asChild>
                  <Button id="pwa-set-geo" type="button" variant="outline" role="combobox" aria-expanded={geoOpen} className="w-52 justify-between font-normal">
                    <span className={cn("truncate", !newSet.geo && "text-muted-foreground")}>
                      {newSet.geo ? `${displayName("region", newSet.geo, locale)} (${newSet.geo})` : t("comments.geoPlaceholder")}
                    </span>
                    <ChevronDownIcon className="size-4 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-64 p-0" align="start">
                  <Command>
                    <CommandInput placeholder={t("comments.geoSearch")} />
                    <CommandList>
                      <CommandEmpty>{t("comments.geoNotFound")}</CommandEmpty>
                      <CommandGroup>
                        {sortedGeos.map((g) => {
                          const name = displayName("region", g.geo, locale);
                          return (
                            <CommandItem
                              key={g.geo}
                              value={`${name} ${g.geo} ${displayName("region", g.geo, "en")}`}
                              data-checked={newSet.geo === g.geo}
                              onSelect={() => {
                                setNewSet({ geo: g.geo, lang: g.langs[0] });
                                setGeoOpen(false);
                              }}
                            >
                              {name} ({g.geo})
                            </CommandItem>
                          );
                        })}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            </div>
            <div className="grid gap-1">
              <Label htmlFor="pwa-set-lang" className="text-xs">{t("comments.lang")}</Label>
              <Select value={newSet.lang} disabled={!newSet.geo} onValueChange={(lang) => setNewSet((s) => ({ ...s, lang }))}>
                <SelectTrigger id="pwa-set-lang" className="w-44">
                  <SelectValue placeholder={t("comments.langPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {geoLangs.map((l) => (
                    <SelectItem key={l} value={l}>
                      {displayName("language", l, locale)} ({l})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button type="button" variant="outline" disabled={!setValid} onClick={addSet}>
              <PlusIcon className="size-3.5" /> {t("comments.addSet")}
            </Button>
          </div>
        </div>

        {!activeSet ? (
          <p className="text-sm text-muted-foreground">{t("comments.noSet")}</p>
        ) : (
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <Button type="button" size="sm" onClick={() => setAdding((v) => !v)} disabled={value.length >= MAX_COMMENTS}>
                <PlusIcon className="size-3.5" /> {t("comments.add")}
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={() => fileRef.current?.click()}>
                <UploadIcon className="size-3.5" /> {t("comments.importCsv")}
              </Button>
              <Button type="button" size="sm" variant="outline" disabled={value.length === 0} onClick={exportCsv}>
                <DownloadIcon className="size-3.5" /> {t("comments.exportCsv")}
              </Button>
              <input
                ref={fileRef}
                type="file"
                accept=".csv,text/csv"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) void importCsv(f);
                }}
              />
              <span className="ml-auto text-xs text-muted-foreground">{t("comments.csvFormat")}</span>
            </div>

            {adding && (
              <div className="grid gap-3 rounded-md border p-3">
                <div className="grid gap-3 sm:grid-cols-[1fr_120px]">
                  <div className="grid gap-1.5">
                    <Label htmlFor="pwa-comment-author">{t("comments.author")}</Label>
                    <Input id="pwa-comment-author" maxLength={100} value={draft.author} onChange={(e) => setDraft((d) => ({ ...d, author: e.target.value }))} />
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="pwa-comment-rating">{t("comments.rating")}</Label>
                    <Input id="pwa-comment-rating" type="number" min={1} max={5} step={1} value={draft.rating} onChange={(e) => setDraft((d) => ({ ...d, rating: Number(e.target.value) }))} />
                  </div>
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="pwa-comment-text">{t("comments.text")}</Label>
                  <Textarea id="pwa-comment-text" rows={3} maxLength={2000} value={draft.text} onChange={(e) => setDraft((d) => ({ ...d, text: e.target.value }))} />
                </div>
                <div className="flex justify-end gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => { setAdding(false); setDraft(EMPTY_DRAFT); }}>
                    {t("comments.cancel")}
                  </Button>
                  <Button type="button" size="sm" disabled={!draftValid} onClick={addComment}>
                    {t("comments.save")}
                  </Button>
                </div>
              </div>
            )}

            <div className="flex items-center justify-between rounded-md border px-3 py-2">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={allSelected}
                  disabled={inSet.length === 0}
                  onCheckedChange={(v) => setSelected(v ? new Set(inSet) : new Set())}
                />
                {t("comments.selectAll")}
              </label>
              <Button type="button" size="sm" variant="outline" disabled={selected.size === 0} onClick={deleteSelected}>
                <Trash2Icon className="size-3.5" /> {t("comments.deleteSelected")}
              </Button>
            </div>

            {inSet.length === 0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">{t("comments.empty")}</p>
            ) : (
              <ul className="flex flex-col divide-y rounded-md border">
                {inSet.map((c, i) => (
                  <li key={i} className="flex items-start gap-3 px-3 py-2">
                    <Checkbox className="mt-1" checked={selected.has(c)} onCheckedChange={() => toggle(c)} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 text-sm font-medium">
                        {c.author}
                        <span className="inline-flex items-center gap-0.5 text-xs text-muted-foreground tabular-nums">
                          <StarIcon className="size-3 fill-current" /> {c.rating}
                        </span>
                      </div>
                      <p className="whitespace-pre-line break-words text-sm text-muted-foreground">{c.text}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
