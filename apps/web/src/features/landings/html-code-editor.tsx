"use client";

import * as React from "react";
import CodeMirror from "@uiw/react-codemirror";
import { html } from "@codemirror/lang-html";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "next-themes";

const extensions = [html()];

/** HTML/Twig source editor with syntax highlighting. Imported only via
 * next/dynamic (landing-template-editor.tsx), so CodeMirror stays out of
 * every other page's bundle. */
export default function HtmlCodeEditor({
  value,
  onChange,
  ariaLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
}) {
  const { resolvedTheme } = useTheme();
  return (
    <CodeMirror
      value={value}
      onChange={onChange}
      extensions={extensions}
      theme={resolvedTheme === "light" ? "light" : oneDark}
      height="100%"
      className="h-full text-xs"
      aria-label={ariaLabel}
      basicSetup={{ foldGutter: true, highlightActiveLine: true }}
    />
  );
}
