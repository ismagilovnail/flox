"use client";

import * as React from "react";
import { CheckCircle2Icon, XCircleIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Caption, Mono } from "@/components/ui/typography";
import { cn } from "@/lib/utils";
import { FUNCTION_SIGNATURES, functionAtCursor, type FormulaValidation } from "@/lib/formula-engine";

export type FormulaInputHandle = { insertAtCursor: (text: string, cursorOffsetFromEnd?: number) => void };

const OPERATORS = ["+", "−", "×", "÷", "(", ")"];
const OPERATOR_INSERT: Record<string, string> = { "−": "-", "×": "*", "÷": "/" };
const FUNCTION_NAMES = Object.keys(FUNCTION_SIGNATURES);

/** `validation` is owned by the parent (computed synchronously from the same
 * `value` in one `useMemo`) and passed in, rather than this component owning
 * its own copy and reporting it up through an effect — a child-effect →
 * parent-state round trip is one render behind, and the parent crashed here
 * once by evaluating a formula against its stale, still-"valid" state right
 * after `value` had already changed to something unparseable mid-keystroke. */
export const FormulaInput = React.forwardRef<
  FormulaInputHandle,
  {
    value: string;
    onChange: (value: string) => void;
    validation: FormulaValidation;
  }
>(function FormulaInput({ value, onChange, validation }, ref) {
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const [cursorPos, setCursorPos] = React.useState(0);

  const insertText = React.useCallback(
    (text: string, cursorOffsetFromEnd = 0) => {
      const el = textareaRef.current;
      const start = el?.selectionStart ?? value.length;
      const end = el?.selectionEnd ?? value.length;
      const next = value.slice(0, start) + text + value.slice(end);
      onChange(next);
      const newPos = start + text.length - cursorOffsetFromEnd;
      requestAnimationFrame(() => {
        el?.focus();
        el?.setSelectionRange(newPos, newPos);
        setCursorPos(newPos);
      });
    },
    [value, onChange],
  );

  React.useImperativeHandle(ref, () => ({ insertAtCursor: insertText }), [insertText]);

  const activeFunction = functionAtCursor(value, cursorPos);
  const activeFunctionSpec = activeFunction ? FUNCTION_SIGNATURES[activeFunction] : null;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1">
        {OPERATORS.map((op) => (
          <Button
            key={op}
            type="button"
            variant="outline"
            size="sm"
            className="h-7 w-7 p-0 font-mono"
            onClick={() => insertText(OPERATOR_INSERT[op] ?? op)}
          >
            {op}
          </Button>
        ))}
        <span className="mx-1 h-4 w-px bg-border" />
        {FUNCTION_NAMES.map((name) => (
          <Button
            key={name}
            type="button"
            variant="outline"
            size="sm"
            className="h-7 font-mono text-xs"
            onClick={() => insertText(`${name}()`, 1)}
          >
            {name}
          </Button>
        ))}
      </div>

      <Textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onSelect={(e) => setCursorPos(e.currentTarget.selectionStart ?? 0)}
        onKeyUp={(e) => setCursorPos(e.currentTarget.selectionStart ?? 0)}
        onClick={(e) => setCursorPos(e.currentTarget.selectionStart ?? 0)}
        placeholder="({revenue} - {cost}) / {clicks}"
        className="min-h-20 font-mono text-sm"
        aria-invalid={!validation.valid}
      />

      {activeFunctionSpec && (
        <Caption className="rounded-md bg-muted px-2 py-1">
          <Mono className="text-xs">{activeFunctionSpec.hint}</Mono> — e.g.{" "}
          <Mono className="text-xs">{activeFunctionSpec.example}</Mono>
        </Caption>
      )}

      <div className={cn("flex items-center gap-1.5 text-xs", validation.valid ? "text-success" : "text-danger")}>
        {validation.valid ? <CheckCircle2Icon className="size-3.5" /> : <XCircleIcon className="size-3.5" />}
        {validation.valid ? "Formula is valid" : validation.error}
      </div>
    </div>
  );
});
