import { CheckIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { FILTER_OPERATOR_I18N_KEY, type FilterOperator } from "@/lib/filters";
import type { FilterTrace, StreamSetEvaluation } from "@/lib/api/routing";

function ResultIcon({ passed }: { passed: boolean }) {
  return passed ? (
    <CheckIcon className="size-3.5 shrink-0 text-success" />
  ) : (
    <XIcon className="size-3.5 shrink-0 text-danger" />
  );
}

function FilterTraceView({ trace, depth = 0, t }: { trace: FilterTrace; depth?: number; t: TFunction }) {
  if (trace.kind === "condition") {
    return (
      <div className={cn("flex items-start gap-1.5 text-xs", depth > 0 && "ml-4")}>
        <ResultIcon passed={trace.passed} />
        <span className="font-mono">
          {trace.field} {t(FILTER_OPERATOR_I18N_KEY[trace.operator as FilterOperator], { ns: "streamSets" })}
          {trace.operator === "BETWEEN" ? ` [${trace.value}, ${trace.valueTo}]` : trace.value ? ` ${trace.value}` : ""}
        </span>
        <span className={cn("text-muted-foreground", !trace.requestValue && "italic")}>
          {t("trace.gotValue", { value: trace.requestValue || t("trace.gotValueEmpty") })}
        </span>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col gap-1", depth > 0 && "ml-4 border-l border-border pl-3")}>
      <div className="flex items-center gap-1.5 text-xs">
        <ResultIcon passed={trace.passed} />
        <Badge variant="outline" className="text-[0.6875rem]">
          {trace.joiner === "AND" ? t("trace.matchAll") : t("trace.matchAny")}
        </Badge>
        {trace.children.length === 0 && (
          <span className="text-muted-foreground">{t("trace.emptyGroupAlwaysMatches")}</span>
        )}
      </div>
      {trace.children.map((child, i) => (
        <FilterTraceView key={i} trace={child} depth={depth + 1} t={t} />
      ))}
    </div>
  );
}

export function StreamSetTraceCard({ evaluation }: { evaluation: StreamSetEvaluation }) {
  const { t } = useTranslation(["routingSimulator", "streamSets"]);
  return (
    <Card size="sm" className={cn("ring-1", evaluation.matched ? "ring-success/40" : "ring-border")}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <span className="font-mono text-xs text-muted-foreground">#{evaluation.priority}</span>
          {evaluation.name}
        </CardTitle>
        <CardAction>
          <Badge variant={evaluation.matched ? "success" : "secondary"}>
            {evaluation.matched ? t("trace.matched") : (evaluation.reasonNotMatched ?? t("trace.notMatchedFallback"))}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <FilterTraceView trace={evaluation.trace} t={t} />
      </CardContent>
    </Card>
  );
}
