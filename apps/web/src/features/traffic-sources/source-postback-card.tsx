"use client";

import { Controller, useWatch, type Control, type FieldErrors, type UseFormGetValues, type UseFormRegister, type UseFormSetValue } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/ui/typography";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MacroPicker } from "@/components/shared/macro-picker";
import { CPA_STATUSES, CPA_STATUS_I18N_KEY } from "@/lib/api/conversions";
import { MACROS } from "@/lib/macros";
import { SOURCE_TEMPLATES, sourceTemplate } from "@/lib/source-templates";
import type { SourceFormValues } from "@/features/traffic-sources/source-form";

const NONE = "none";
const TEMPLATE_URLS = new Set(SOURCE_TEMPLATES.map((t) => t.postbackUrl));
/** What a postback to a source can carry: the conversion and the click. */
const POSTBACK_MACROS = MACROS.filter((m) => ["{external_click_id}", "{click_id}", "{status}", "{revenue}", "{currency}", "{country}"].includes(m.token));

/** The network template and the postback to the source (audit 2026-10-15,
 * INT-1). Picking a template fills the postback with the network's format;
 * the account-specific REPLACE_… parts are the operator's to fill in. The
 * tracking link builder then offers the template's macros. */
export function SourcePostbackCard({
  control,
  register,
  setValue,
  getValues,
  errors,
  disabled,
}: {
  control: Control<SourceFormValues>;
  register: UseFormRegister<SourceFormValues>;
  setValue: UseFormSetValue<SourceFormValues>;
  getValues: UseFormGetValues<SourceFormValues>;
  errors: FieldErrors<SourceFormValues>;
  disabled: boolean;
}) {
  const { t } = useTranslation(["trafficSources", "conversions"]);
  const templateKey = useWatch({ control, name: "templateKey" });
  const template = sourceTemplate(templateKey);

  function pickTemplate(key: string) {
    const next = key === NONE ? "" : key;
    setValue("templateKey", next, { shouldDirty: true });
    const current = getValues("postbackUrl").trim();
    // Only a postback still empty or still another template's is replaced.
    const tpl = sourceTemplate(next);
    if (tpl && (current === "" || TEMPLATE_URLS.has(current))) {
      setValue("postbackUrl", tpl.postbackUrl, { shouldDirty: true, shouldValidate: true });
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("postback.title")}</CardTitle>
        <CardDescription>{t("postback.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid gap-1.5 sm:max-w-xs">
          <Label htmlFor="src-template-key">{t("postback.templateLabel")}</Label>
          <Select value={templateKey || NONE} onValueChange={pickTemplate} disabled={disabled}>
            <SelectTrigger id="src-template-key" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>{t("postback.noTemplate")}</SelectItem>
              {SOURCE_TEMPLATES.map((tpl) => (
                <SelectItem key={tpl.key} value={tpl.key}>
                  {tpl.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {template && (
          <div className="flex flex-col gap-1.5 rounded-md border border-border p-3 text-xs">
            <p>{t("postback.linkHint", { name: template.name })}</p>
            <Mono className="break-all text-xs">
              external_click_id={template.link.external_click_id}
              {Object.entries(template.subs).map(([sub, what]) => ` · ${sub}=${template.link[sub]} (${what})`)}
            </Mono>
            {template.caution && (
              <p className="flex items-start gap-1.5 text-warning">
                <TriangleAlert aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
                {t(`postback.caution.${template.caution}`)}
              </p>
            )}
            <p className="text-muted-foreground">
              {t("postback.sources")}{" "}
              {template.sources.map((url, i) => (
                <a key={url} href={url} target="_blank" rel="noreferrer" className="text-primary underline-offset-4 hover:underline">
                  [{i + 1}]
                </a>
              ))}
            </p>
          </div>
        )}

        <div className="grid gap-1.5">
          <div className="flex items-center justify-between">
            <Label htmlFor="src-postback-url">{t("postback.urlLabel")}</Label>
            <MacroPicker macros={POSTBACK_MACROS} onInsert={(token) => setValue("postbackUrl", `${getValues("postbackUrl")}${token}`, { shouldDirty: true })} />
          </div>
          <Input
            id="src-postback-url"
            placeholder="https://network.example/postback?clickid={external_click_id}&payout={revenue}"
            className="font-mono text-xs"
            {...register("postbackUrl")}
            aria-invalid={!!errors.postbackUrl}
            disabled={disabled}
          />
          {errors.postbackUrl && <p className="text-xs text-danger">{errors.postbackUrl.message}</p>}
          <p className="text-xs text-muted-foreground">{t("postback.urlHint")}</p>
        </div>

        <div className="grid gap-1.5">
          <span className="text-sm font-medium">{t("postback.statusesLabel")}</span>
          <Controller
            control={control}
            name="postbackStatuses"
            render={({ field }) => (
              <div className="flex flex-wrap gap-4">
                {CPA_STATUSES.map((st) => (
                  <label key={st} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={field.value.includes(st)}
                      disabled={disabled}
                      onCheckedChange={(on) => field.onChange(on ? [...field.value, st] : field.value.filter((v) => v !== st))}
                    />
                    {t(CPA_STATUS_I18N_KEY[st], { ns: "conversions" })}
                  </label>
                ))}
              </div>
            )}
          />
          {errors.postbackStatuses && <p className="text-xs text-danger">{errors.postbackStatuses.message}</p>}
          <p className="text-xs text-muted-foreground">{t("postback.statusesHint")}</p>
        </div>
      </CardContent>
    </Card>
  );
}
