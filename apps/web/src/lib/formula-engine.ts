/**
 * Custom Metrics formula engine (§30.5) — a real tokenizer/parser/evaluator
 * over registry metric tokens, not a `Function()`/`eval` shortcut. Division
 * is safe everywhere (bare `/` and `DIV()` alike): division by zero or a
 * null operand yields `null` ("empty"), never a thrown error — mandatory
 * per spec, not optional. `null` propagates through every other operator
 * and function the same way (except IF/EMPTYIF, which use it explicitly).
 */

/** Every way a formula can be rejected. The UI translates the code
 * (customMetrics:formulaErrors.<code>, with params); `message` stays the
 * English text for logs and tests. */
export type FormulaErrorCode =
  | "unterminatedToken"
  | "emptyToken"
  | "invalidNumber"
  | "unexpectedChar"
  | "expectedClose"
  | "expectedOpenAfter"
  | "unexpected"
  | "unknownFunction"
  | "arity"
  | "unexpectedEnd"
  | "empty"
  | "invalid"
  | "noMetric"
  | "unknownMetric"
  | "notInsertable"
  | "mixedSources";

export type FormulaErrorParams = Record<string, string | number>;

const ENGLISH: Record<FormulaErrorCode, (p: FormulaErrorParams) => string> = {
  unterminatedToken: () => "Unterminated metric token — missing closing }",
  emptyToken: () => "Empty metric token {}",
  invalidNumber: (p) => `Invalid number "${p.value}"`,
  unexpectedChar: (p) => `Unexpected character "${p.value}"`,
  expectedClose: () => "Expected ')'",
  expectedOpenAfter: (p) => `Expected '(' after ${p.name}`,
  unexpected: (p) => `Unexpected "${p.value}"`,
  unknownFunction: (p) => `Unknown function "${p.name}"`,
  arity: (p) => `${p.name} expects ${p.arity} argument(s), got ${p.got}`,
  unexpectedEnd: () => "Formula ends unexpectedly — missing an operand",
  empty: () => "Formula is empty",
  invalid: () => "Invalid formula",
  noMetric: () => "Formula must reference at least one metric",
  unknownMetric: (p) => `Unknown metric {${p.id}}`,
  notInsertable: (p) => `"${p.label}" can't be used in custom metric formulas`,
  mixedSources: (p) => `A formula can only draw from one data source — this mixes ${p.sources}`,
};

export class FormulaError extends Error {
  pos: number;
  code: FormulaErrorCode;
  params: FormulaErrorParams;
  constructor(code: FormulaErrorCode, params: FormulaErrorParams, pos: number) {
    super(ENGLISH[code](params));
    this.code = code;
    this.params = params;
    this.pos = pos;
  }
}

type TokenType =
  | "number"
  | "metric"
  | "ident"
  | "lparen"
  | "rparen"
  | "comma"
  | "plus"
  | "minus"
  | "star"
  | "slash"
  | "gt"
  | "lt"
  | "ge"
  | "le"
  | "eq"
  | "ne"
  | "eof";

type Token = { type: TokenType; value: string; pos: number };

function tokenize(input: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < input.length) {
    const ch = input[i];
    if (/\s/.test(ch)) {
      i++;
      continue;
    }
    if (ch === "{") {
      const end = input.indexOf("}", i);
      if (end === -1) throw new FormulaError("unterminatedToken", {}, i);
      const name = input.slice(i + 1, end).trim();
      if (!name) throw new FormulaError("emptyToken", {}, i);
      tokens.push({ type: "metric", value: name, pos: i });
      i = end + 1;
      continue;
    }
    if (/[0-9.]/.test(ch)) {
      let j = i;
      while (j < input.length && /[0-9.]/.test(input[j])) j++;
      const raw = input.slice(i, j);
      if (!/^\d+(\.\d+)?$/.test(raw)) throw new FormulaError("invalidNumber", { value: raw }, i);
      tokens.push({ type: "number", value: raw, pos: i });
      i = j;
      continue;
    }
    if (/[a-zA-Z_]/.test(ch)) {
      let j = i;
      while (j < input.length && /[a-zA-Z0-9_]/.test(input[j])) j++;
      tokens.push({ type: "ident", value: input.slice(i, j), pos: i });
      i = j;
      continue;
    }
    if (ch === "(") {
      tokens.push({ type: "lparen", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === ")") {
      tokens.push({ type: "rparen", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === ",") {
      tokens.push({ type: "comma", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === "+") {
      tokens.push({ type: "plus", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === "-") {
      tokens.push({ type: "minus", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === "*" || ch === "×") {
      tokens.push({ type: "star", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === "/" || ch === "÷") {
      tokens.push({ type: "slash", value: ch, pos: i });
      i++;
      continue;
    }
    if (ch === ">") {
      if (input[i + 1] === "=") {
        tokens.push({ type: "ge", value: ">=", pos: i });
        i += 2;
      } else {
        tokens.push({ type: "gt", value: ">", pos: i });
        i++;
      }
      continue;
    }
    if (ch === "<") {
      if (input[i + 1] === "=") {
        tokens.push({ type: "le", value: "<=", pos: i });
        i += 2;
      } else {
        tokens.push({ type: "lt", value: "<", pos: i });
        i++;
      }
      continue;
    }
    if (ch === "=" && input[i + 1] === "=") {
      tokens.push({ type: "eq", value: "==", pos: i });
      i += 2;
      continue;
    }
    if (ch === "!" && input[i + 1] === "=") {
      tokens.push({ type: "ne", value: "!=", pos: i });
      i += 2;
      continue;
    }
    throw new FormulaError("unexpectedChar", { value: ch }, i);
  }
  tokens.push({ type: "eof", value: "", pos: input.length });
  return tokens;
}

type BinaryOp = "+" | "-" | "*" | "/" | ">" | "<" | ">=" | "<=" | "==" | "!=";

type Node =
  | { kind: "number"; value: number }
  | { kind: "metric"; id: string }
  | { kind: "unary"; arg: Node }
  | { kind: "binary"; op: BinaryOp; left: Node; right: Node }
  | { kind: "call"; name: string; args: Node[] };

export const FUNCTION_SIGNATURES: Record<string, { min: number; max: number; hint: string; example: string }> = {
  DIV: { min: 2, max: 2, hint: "DIV(numerator, denominator)", example: "DIV({revenue}, {cost})" },
  EMPTYIF: { min: 2, max: 2, hint: "EMPTYIF(value, valueToTreatAsEmpty)", example: "EMPTYIF({cost}, 0)" },
  IF: { min: 3, max: 3, hint: "IF(condition, ifTrue, ifFalse)", example: "IF({clicks} > 0, {revenue}, 0)" },
  ROUND: { min: 1, max: 2, hint: "ROUND(value, decimals?)", example: "ROUND({roi}, 1)" },
  ABS: { min: 1, max: 1, hint: "ABS(value)", example: "ABS({profit})" },
  MIN: { min: 1, max: Infinity, hint: "MIN(value, ...)", example: "MIN({cpc}, {cpa})" },
  MAX: { min: 1, max: Infinity, hint: "MAX(value, ...)", example: "MAX({cpc}, {cpa})" },
};

class Parser {
  tokens: Token[];
  pos = 0;
  constructor(tokens: Token[]) {
    this.tokens = tokens;
  }
  peek() {
    return this.tokens[this.pos];
  }
  next() {
    return this.tokens[this.pos++];
  }
  expect(type: TokenType, code: "expectedClose" | "expectedOpenAfter", params: FormulaErrorParams = {}) {
    const t = this.next();
    if (t.type !== type) throw new FormulaError(code, params, t.pos);
    return t;
  }

  parse(): Node {
    const node = this.parseComparison();
    if (this.peek().type !== "eof") throw new FormulaError("unexpected", { value: this.peek().value }, this.peek().pos);
    return node;
  }

  parseComparison(): Node {
    const left = this.parseAdd();
    const compareOps: Partial<Record<TokenType, BinaryOp>> = {
      gt: ">",
      lt: "<",
      ge: ">=",
      le: "<=",
      eq: "==",
      ne: "!=",
    };
    const op = compareOps[this.peek().type];
    if (op) {
      this.next();
      const right = this.parseAdd();
      return { kind: "binary", op, left, right };
    }
    return left;
  }

  parseAdd(): Node {
    let left = this.parseMul();
    while (this.peek().type === "plus" || this.peek().type === "minus") {
      const op: BinaryOp = this.next().type === "plus" ? "+" : "-";
      const right = this.parseMul();
      left = { kind: "binary", op, left, right };
    }
    return left;
  }

  parseMul(): Node {
    let left = this.parseUnary();
    while (this.peek().type === "star" || this.peek().type === "slash") {
      const op: BinaryOp = this.next().type === "star" ? "*" : "/";
      const right = this.parseUnary();
      left = { kind: "binary", op, left, right };
    }
    return left;
  }

  parseUnary(): Node {
    if (this.peek().type === "minus") {
      this.next();
      return { kind: "unary", arg: this.parseUnary() };
    }
    return this.parseAtom();
  }

  parseAtom(): Node {
    const t = this.peek();
    if (t.type === "number") {
      this.next();
      return { kind: "number", value: Number(t.value) };
    }
    if (t.type === "metric") {
      this.next();
      return { kind: "metric", id: t.value };
    }
    if (t.type === "lparen") {
      this.next();
      const inner = this.parseComparison();
      this.expect("rparen", "expectedClose");
      return inner;
    }
    if (t.type === "ident") {
      this.next();
      const name = t.value.toUpperCase();
      const spec = FUNCTION_SIGNATURES[name];
      if (!spec) throw new FormulaError("unknownFunction", { name: t.value }, t.pos);
      this.expect("lparen", "expectedOpenAfter", { name });
      const args: Node[] = [];
      if (this.peek().type !== "rparen") {
        args.push(this.parseComparison());
        while (this.peek().type === "comma") {
          this.next();
          args.push(this.parseComparison());
        }
      }
      this.expect("rparen", "expectedClose");
      if (args.length < spec.min || args.length > spec.max) {
        const arity = spec.min === spec.max ? `${spec.min}` : `${spec.min}-${spec.max === Infinity ? "many" : spec.max}`;
        throw new FormulaError("arity", { name, arity, got: args.length }, t.pos);
      }
      return { kind: "call", name, args };
    }
    if (t.type === "eof") throw new FormulaError("unexpectedEnd", {}, t.pos);
    throw new FormulaError("unexpected", { value: t.value }, t.pos);
  }
}

export function parseFormula(input: string): Node {
  return new Parser(tokenize(input)).parse();
}

function safeDiv(a: number | null, b: number | null): number | null {
  if (a === null || b === null || b === 0) return null;
  return a / b;
}

export function evaluateNode(node: Node, values: Record<string, number | null>): number | null {
  switch (node.kind) {
    case "number":
      return node.value;
    case "metric": {
      const v = values[node.id];
      return v === undefined ? null : v;
    }
    case "unary": {
      const v = evaluateNode(node.arg, values);
      return v === null ? null : -v;
    }
    case "binary": {
      const l = evaluateNode(node.left, values);
      const r = evaluateNode(node.right, values);
      switch (node.op) {
        case "+":
          return l === null || r === null ? null : l + r;
        case "-":
          return l === null || r === null ? null : l - r;
        case "*":
          return l === null || r === null ? null : l * r;
        case "/":
          return safeDiv(l, r);
        default:
          if (l === null || r === null) return null;
          switch (node.op) {
            case ">":
              return l > r ? 1 : 0;
            case "<":
              return l < r ? 1 : 0;
            case ">=":
              return l >= r ? 1 : 0;
            case "<=":
              return l <= r ? 1 : 0;
            case "==":
              return l === r ? 1 : 0;
            case "!=":
              return l !== r ? 1 : 0;
          }
          return null;
      }
    }
    case "call": {
      const args = node.args.map((a) => evaluateNode(a, values));
      switch (node.name) {
        case "DIV":
          return safeDiv(args[0], args[1]);
        case "EMPTYIF":
          return args[0] !== null && args[0] === args[1] ? null : args[0];
        case "IF":
          return args[0] === null ? null : args[0] !== 0 ? args[1] : args[2];
        case "ROUND": {
          if (args[0] === null) return null;
          const digits = args[1] ?? 0;
          if (digits === null) return null;
          const factor = 10 ** digits;
          return Math.round(args[0] * factor) / factor;
        }
        case "ABS":
          return args[0] === null ? null : Math.abs(args[0]);
        case "MIN":
          return args.some((a) => a === null) ? null : Math.min(...(args as number[]));
        case "MAX":
          return args.some((a) => a === null) ? null : Math.max(...(args as number[]));
        default:
          return null;
      }
    }
  }
}

function collectMetricIds(node: Node, out: Set<string>) {
  switch (node.kind) {
    case "metric":
      out.add(node.id);
      return;
    case "unary":
      collectMetricIds(node.arg, out);
      return;
    case "binary":
      collectMetricIds(node.left, out);
      collectMetricIds(node.right, out);
      return;
    case "call":
      node.args.forEach((a) => collectMetricIds(a, out));
      return;
    default:
      return;
  }
}

export type FormulaRegistryEntry = {
  id: string;
  label: string;
  dataSource: string;
  insertable: boolean;
};

export type FormulaValidation =
  | { valid: true; error?: undefined; usedMetricIds: string[]; dataSource: string }
  | {
      valid: false;
      /** English, for logs; the UI renders code + params. */
      error: string;
      code: FormulaErrorCode;
      params: FormulaErrorParams;
      usedMetricIds?: undefined;
      dataSource?: undefined;
    };

function invalid(code: FormulaErrorCode, params: FormulaErrorParams = {}): FormulaValidation {
  return { valid: false, error: ENGLISH[code](params), code, params };
}

/** Validates a formula against the metric catalog: syntax, unknown tokens, the
 * single-data-source constraint, and the LTV-metrics-forbidden constraint. */
export function validateFormula(formula: string, registry: FormulaRegistryEntry[]): FormulaValidation {
  const trimmed = formula.trim();
  if (!trimmed) return invalid("empty");

  let ast: Node;
  try {
    ast = parseFormula(trimmed);
  } catch (e) {
    return e instanceof FormulaError ? invalid(e.code, e.params) : invalid("invalid");
  }

  const idsSet = new Set<string>();
  collectMetricIds(ast, idsSet);
  const usedMetricIds = [...idsSet];
  if (usedMetricIds.length === 0) return invalid("noMetric");

  const dataSources = new Set<string>();
  for (const id of usedMetricIds) {
    const meta = registry.find((m) => m.id === id);
    if (!meta) return invalid("unknownMetric", { id });
    if (!meta.insertable) return invalid("notInsertable", { id, label: meta.label });
    dataSources.add(meta.dataSource);
  }
  if (dataSources.size > 1) {
    return invalid("mixedSources", { sources: [...dataSources].join(" + ") });
  }

  return { valid: true, usedMetricIds, dataSource: [...dataSources][0] };
}

export function evaluateFormula(formula: string, values: Record<string, number | null>): number | null {
  return evaluateNode(parseFormula(formula), values);
}

/** For the formula input's "cursor is inside a function call" hint. Returns the
 * function name whose argument list currently contains `cursorPos`, if any. */
export function functionAtCursor(input: string, cursorPos: number): string | null {
  let depth = 0;
  const stack: string[] = [];
  let i = 0;
  let identStart = -1;
  while (i < input.length && i < cursorPos) {
    const ch = input[i];
    if (/[a-zA-Z_]/.test(ch)) {
      if (identStart === -1) identStart = i;
    } else {
      if (ch === "(") {
        const name = identStart !== -1 ? input.slice(identStart, i) : "";
        stack.push(name.toUpperCase());
        depth++;
      } else if (ch === ")") {
        stack.pop();
        depth = Math.max(0, depth - 1);
      }
      identStart = -1;
    }
    i++;
  }
  const top = stack[stack.length - 1];
  return depth > 0 && top && top in FUNCTION_SIGNATURES ? top : null;
}
