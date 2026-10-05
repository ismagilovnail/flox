"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { toast } from "sonner";
import { Loader2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useUpdateLanding } from "@/hooks/use-landings";
import { previewLanding, type Landing } from "@/lib/api/landings";

const HtmlCodeEditor = dynamic(() => import("@/features/landings/html-code-editor"), {
  ssr: false,
  loading: () => (
    <div className="flex h-full items-center justify-center text-muted-foreground">
      <Loader2Icon className="size-5 animate-spin" />
    </div>
  ),
});

const PREVIEW_DEBOUNCE_MS = 400;

/** Full-screen template editor: the entry file's source on the left, a
 * live preview on the right. The preview HTML comes from the API's
 * `/landings/{id}/preview` — the same Go renderer the tracker serves
 * visitors with (base URL, `{{ trackUrl }}` macros, injected runtime), so
 * nothing is re-implemented here. It runs in an iframe sandboxed WITHOUT
 * allow-same-origin: operator-uploaded scripts execute, but on an opaque
 * origin with no access to this app's cookies or storage. */
export function LandingTemplateEditor({
  landing,
  open,
  onOpenChange,
}: {
  landing: Landing;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation(["landings", "common"]);
  const update = useUpdateLanding(landing.id);
  const [source, setSource] = React.useState(landing.content);
  const [confirmDiscard, setConfirmDiscard] = React.useState(false);
  const dirty = source !== landing.content;

  // Reset to the saved template every time the dialog opens.
  const [prevOpen, setPrevOpen] = React.useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) setSource(landing.content);
  }

  const preview = useDebouncedPreview(landing.id, source, open);

  function requestClose(next: boolean) {
    if (!next && dirty) {
      setConfirmDiscard(true);
      return;
    }
    onOpenChange(next);
  }

  function save() {
    update.mutate(
      { content: source },
      {
        onSuccess: () => toast(t("editor.saved"), { description: landing.name }),
        onError: (err) => toast.error(t("editor.saveError"), { description: err.message }),
      },
    );
  }

  return (
    <>
      <Dialog open={open} onOpenChange={requestClose}>
        <DialogContent className="flex h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] flex-col gap-3 sm:max-w-[calc(100vw-2rem)]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {t("editor.title", { name: landing.name })}
              <Badge variant="outline" className="font-mono">
                {landing.entryFile}
              </Badge>
              {dirty && <Badge variant="warning">{t("editor.unsaved")}</Badge>}
            </DialogTitle>
            <DialogDescription>{t("editor.description")}</DialogDescription>
          </DialogHeader>

          <div className="grid min-h-0 flex-1 gap-3 lg:grid-cols-2">
            <div className="min-h-64 overflow-hidden rounded-md border">
              <HtmlCodeEditor value={source} onChange={setSource} ariaLabel={t("editor.codeAria")} />
            </div>
            <div className="relative min-h-64 overflow-hidden rounded-md border bg-white">
              {preview.error ? (
                <div className="flex h-full items-center justify-center p-4 text-center text-sm text-danger">
                  {t("editor.previewError")}: {preview.error}
                </div>
              ) : (
                <iframe
                  title={t("editor.previewTitle")}
                  sandbox="allow-scripts"
                  srcDoc={preview.html}
                  className="size-full"
                />
              )}
              {preview.loading && (
                <Loader2Icon className="absolute top-2 right-2 size-4 animate-spin text-muted-foreground" />
              )}
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => requestClose(false)}>
              {t("actions.close", { ns: "common" })}
            </Button>
            <Button onClick={save} disabled={!dirty || update.isPending || source.trim() === ""}>
              {update.isPending && <Loader2Icon className="size-4 animate-spin" />}
              {t("editor.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={confirmDiscard} onOpenChange={setConfirmDiscard}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("editor.discardTitle")}</DialogTitle>
            <DialogDescription>{t("editor.discardDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDiscard(false)}>
              {t("editor.keepEditing")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmDiscard(false);
                onOpenChange(false);
              }}
            >
              {t("editor.discard")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

/** Re-renders the preview PREVIEW_DEBOUNCE_MS after the last keystroke;
 * a response for an older source is dropped if a newer request started. */
function useDebouncedPreview(id: string, source: string, enabled: boolean) {
  const [state, setState] = React.useState<{ html: string; error: string | null; loading: boolean }>({
    html: "",
    error: null,
    loading: false,
  });
  const latest = React.useRef(0);

  React.useEffect(() => {
    if (!enabled) return;
    const request = ++latest.current;
    const timer = window.setTimeout(() => {
      setState((s) => ({ ...s, loading: true }));
      previewLanding(id, source).then(
        (res) => {
          if (request === latest.current) setState({ html: res.html, error: null, loading: false });
        },
        (err: Error) => {
          if (request === latest.current) setState((s) => ({ ...s, error: err.message, loading: false }));
        },
      );
    }, PREVIEW_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [id, source, enabled]);

  return state;
}
