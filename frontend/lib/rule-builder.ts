import type { Schemas } from "@/lib/api/client";

/**
 * The no-code rule builder (docs/CONSOLE.md, Rules): a rule is WHEN (the kinds of event it
 * looks at), IF (conditions joined by "and" or "or", optionally in groups) and THEN (the risk
 * it adds). This module turns that form into the engine's condition tree and back, and says it
 * in plain words. People never see the tree.
 */

export type CatalogField = Schemas["RiskCatalog"]["fields"][number];
export type Op = "eq" | "ne" | "gt" | "gte" | "lt" | "lte" | "in" | "not_in";
export type Match = "all" | "any";
export type Value = number | string | boolean | string[];

export interface Clause {
  kind: "clause";
  id: string;
  field: string;
  op: Op;
  /** null until the person fills it in. Money is held in major units (naira, not kobo). */
  value: Value | null;
}
export interface Group {
  kind: "group";
  id: string;
  match: Match;
  clauses: Clause[];
}
export type Item = Clause | Group;

export interface Draft {
  /** Event types the rule looks at; empty means every event. */
  when: string[];
  match: Match;
  items: Item[];
}

/** The engine's condition tree (RuleCondition). */
export type Tree =
  | { all: Tree[] }
  | { any: Tree[] }
  | { not: Tree }
  | { field: string; op: Op; value: unknown };

const EVENT_TYPE = "event.type";
/** Money facts are minor units (kobo); people type major units (naira). */
const MINOR = 100;

let seq = 0;
export const newId = () => `b${++seq}`;

export function newClause(field = "", op: Op = "eq"): Clause {
  return { kind: "clause", id: newId(), field, op, value: null };
}

export function newGroup(): Group {
  return { kind: "group", id: newId(), match: "any", clauses: [newClause()] };
}

export function emptyDraft(): Draft {
  return { when: ["transaction"], match: "all", items: [newClause()] };
}

/** The operators people pick from for a field, in words. */
export function operators(
  f: CatalogField | undefined,
): { op: Op; label: string }[] {
  if (!f) return [];
  if (f.type === "boolean") return [{ op: "eq", label: "is" }];
  if (f.type === "number")
    return [
      { op: "gt", label: "is more than" },
      { op: "gte", label: "is at least" },
      { op: "lt", label: "is less than" },
      { op: "lte", label: "is at most" },
      { op: "eq", label: "is exactly" },
      { op: "ne", label: "is not" },
    ];
  return [
    { op: "in", label: "is" },
    { op: "not_in", label: "is not" },
  ];
}

/** A clause's field as the engine compares it, or why it cannot be saved yet. */
function compare(c: Clause, fields: Map<string, CatalogField>): Tree | string {
  const f = fields.get(c.field);
  if (!f) return "Choose what to check.";
  const v = c.value;
  if (v === null || v === "" || (Array.isArray(v) && v.length === 0))
    return `Fill in a value for “${f.label}”.`;
  if (f.type === "number") {
    if (typeof v !== "number" || !Number.isFinite(v))
      return `“${f.label}” needs a number.`;
    const n = f.unit === "money" ? Math.round(v * MINOR) : v;
    return { field: f.name, op: c.op, value: n };
  }
  if (f.type === "boolean")
    return { field: f.name, op: "eq", value: v === true };
  const list = Array.isArray(v) ? v : [String(v)];
  if (list.length === 1)
    return {
      field: f.name,
      op: c.op === "not_in" ? "ne" : "eq",
      value: list[0],
    };
  return {
    field: f.name,
    op: c.op === "not_in" ? "not_in" : "in",
    value: list,
  };
}

/** The condition tree of a draft, or the first problem that stops it being saved. */
export function toCondition(
  d: Draft,
  catalog: CatalogField[],
): { tree: Tree } | { problem: string } {
  const fields = new Map(catalog.map((f) => [f.name, f]));
  const body: Tree[] = [];
  for (const it of d.items) {
    if (it.kind === "clause") {
      const t = compare(it, fields);
      if (typeof t === "string") return { problem: t };
      body.push(t);
      continue;
    }
    if (it.clauses.length === 0) continue;
    const ts: Tree[] = [];
    for (const c of it.clauses) {
      const t = compare(c, fields);
      if (typeof t === "string") return { problem: t };
      ts.push(t);
    }
    body.push(
      ts.length === 1 ? ts[0]! : it.match === "all" ? { all: ts } : { any: ts },
    );
  }
  if (body.length === 0) return { problem: "Add at least one condition." };
  const when: Tree | null =
    d.when.length === 0
      ? null
      : d.when.length === 1
        ? { field: EVENT_TYPE, op: "eq", value: d.when[0] }
        : { field: EVENT_TYPE, op: "in", value: d.when };
  const root: Tree =
    body.length === 1
      ? body[0]!
      : d.match === "all"
        ? { all: body }
        : { any: body };
  if (!when) return { tree: root };
  return {
    tree: "all" in root ? { all: [when, ...root.all] } : { all: [when, root] },
  };
}

function isCompare(t: unknown): t is { field: string; op: Op; value: unknown } {
  return typeof t === "object" && t !== null && "field" in t && "op" in t;
}

function clauseOf(
  t: unknown,
  fields: Map<string, CatalogField>,
): Clause | null {
  if (!isCompare(t)) return null;
  const f = fields.get(t.field);
  if (!f) return null;
  let op = t.op;
  let value: Value;
  if (f.type === "number") {
    if (typeof t.value !== "number" || op === "in" || op === "not_in")
      return null;
    value = f.unit === "money" ? t.value / MINOR : t.value;
  } else if (f.type === "boolean") {
    if (typeof t.value !== "boolean") return null;
    // "is not yes" reads as "is no".
    value = op === "ne" ? !t.value : t.value;
    op = "eq";
  } else {
    const list = Array.isArray(t.value)
      ? t.value.map(String)
      : [String(t.value)];
    value = list;
    op = op === "ne" || op === "not_in" ? "not_in" : "in";
  }
  return { kind: "clause", id: newId(), field: f.name, op, value };
}

function itemOf(t: unknown, fields: Map<string, CatalogField>): Item | null {
  const c = clauseOf(t, fields);
  if (c) return c;
  if (typeof t !== "object" || t === null) return null;
  const match: Match | null = "all" in t ? "all" : "any" in t ? "any" : null;
  if (!match) return null;
  const list = (t as Record<Match, unknown[]>)[match];
  if (!Array.isArray(list)) return null;
  const clauses = list.map((x) => clauseOf(x, fields));
  if (clauses.some((x) => x === null)) return null;
  return { kind: "group", id: newId(), match, clauses: clauses as Clause[] };
}

/**
 * The draft for a stored condition, or null when it uses logic the builder does not show (a
 * "not", or groups inside groups): such a rule is described in words and changed through the
 * API.
 */
export function fromCondition(
  tree: unknown,
  catalog: CatalogField[],
): Draft | null {
  const fields = new Map(catalog.map((f) => [f.name, f]));
  let when: string[] = [];
  let rest: unknown[];
  let match: Match = "all";
  const isWhen = (t: unknown) =>
    isCompare(t) && t.field === EVENT_TYPE && (t.op === "eq" || t.op === "in");
  if (typeof tree !== "object" || tree === null) return null;
  if ("all" in tree && Array.isArray(tree.all)) {
    const w = tree.all.findIndex(isWhen);
    rest = [...tree.all];
    if (w >= 0) {
      const t = rest.splice(w, 1)[0] as { value: unknown };
      when = Array.isArray(t.value) ? t.value.map(String) : [String(t.value)];
    }
    // all(when, any(…)) is "when …, if any of these".
    const only = rest[0];
    if (
      rest.length === 1 &&
      typeof only === "object" &&
      only !== null &&
      "any" in only &&
      Array.isArray(only.any)
    ) {
      match = "any";
      rest = only.any;
    }
  } else if ("any" in tree && Array.isArray(tree.any)) {
    match = "any";
    rest = tree.any;
  } else if (isWhen(tree)) {
    const v = (tree as { value: unknown }).value;
    when = Array.isArray(v) ? v.map(String) : [String(v)];
    rest = [];
  } else {
    rest = [tree];
  }
  const items = rest.map((t) => itemOf(t, fields));
  if (items.some((x) => x === null)) return null;
  return { when, match, items: items as Item[] };
}

const countryName = (() => {
  try {
    const names = new Intl.DisplayNames(["en"], { type: "region" });
    return (code: string) => names.of(code) ?? code;
  } catch {
    return (code: string) => code;
  }
})();

/** A value in words: choices by label, countries by name, money with two decimals. */
export function valueText(
  f: CatalogField | undefined,
  v: Value | null,
): string {
  if (v === null || !f) return "…";
  if (typeof v === "boolean") return v ? "yes" : "no";
  if (typeof v === "number") {
    if (f.unit === "money")
      return v.toLocaleString("en", {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });
    if (f.unit === "ratio") return `${v.toLocaleString("en")} times`;
    return v.toLocaleString("en");
  }
  const list = (Array.isArray(v) ? v : [v]).map((x) => {
    const choice = f.values?.find((c) => c.value === x);
    if (choice) return choice.label.toLowerCase();
    return f.kind === "country" ? countryName(x) : x;
  });
  if (list.length <= 1) return list[0] ?? "…";
  return `${list.slice(0, -1).join(", ")} or ${list.at(-1)}`;
}

const lower = (s: string) => s.charAt(0).toLowerCase() + s.slice(1);

/** One condition in words: "amount is more than 1,000,000.00". */
export function clauseText(
  c: Clause,
  fields: Map<string, CatalogField>,
): string {
  const f = fields.get(c.field);
  if (!f) return "…";
  const op = operators(f).find((o) => o.op === c.op)?.label ?? "is";
  return `${lower(f.label)} ${op} ${valueText(f, c.value)}`;
}

/** What kinds of event a rule looks at, in words. */
export function whenText(when: string[], catalog: CatalogField[]): string {
  if (when.length === 0) return "any event";
  const types = catalog.find((f) => f.name === EVENT_TYPE)?.values ?? [];
  const names = when.map(
    (w) => types.find((t) => t.value === w)?.label.toLowerCase() ?? w,
  );
  const list =
    names.length === 1
      ? names[0]!
      : `${names.slice(0, -1).join(", ")} or ${names.at(-1)}`;
  return `a ${list}`;
}

/** The whole rule as one sentence: "When a transaction happens, if … , add 40 risk points." */
export function describe(
  d: Draft,
  catalog: CatalogField[],
  then?: { weight: number; severity: string },
): string {
  const fields = new Map(catalog.map((f) => [f.name, f]));
  const join = (m: Match) => (m === "all" ? " and " : " or ");
  const parts = d.items.map((it) =>
    it.kind === "clause"
      ? clauseText(it, fields)
      : `(${it.clauses.map((c) => clauseText(c, fields)).join(join(it.match))})`,
  );
  const cond = parts.length ? parts.join(join(d.match)) : "…";
  const action = then
    ? `, add ${then.weight} risk point${then.weight === 1 ? "" : "s"} (${then.severity} severity)`
    : "";
  return `When ${whenText(d.when, catalog)} happens, if ${cond}${action}.`;
}

/** Whether a field can be on any of the chosen events (an empty choice means every event). */
export function fieldFits(f: CatalogField, when: string[]): boolean {
  return (
    f.events.length === 0 ||
    when.length === 0 ||
    f.events.some((e) => when.includes(e))
  );
}

/** A rule code from its name: lowercase words joined by _, unique among taken codes. */
export function codeFromName(name: string, taken: Set<string>): string {
  let base = name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 56);
  if (!/^[a-z]/.test(base)) base = `rule_${base}`.replace(/_+$/, "");
  if (base.length < 3) base = `${base}_rule`;
  let code = base;
  for (let i = 2; taken.has(code); i++) code = `${base}_${i}`;
  return code;
}

/** Any stored condition in words, for a rule the builder cannot show. */
export function treeText(tree: unknown, catalog: CatalogField[]): string {
  const fields = new Map(catalog.map((f) => [f.name, f]));
  const go = (n: unknown): string => {
    const c = clauseOf(n, fields);
    if (c) return clauseText(c, fields);
    if (isCompare(n))
      return `${fields.get(n.field)?.label.toLowerCase() ?? "a fact"} matches`;
    if (typeof n !== "object" || n === null) return "…";
    if ("not" in n) return `not ${go(n.not)}`;
    const list = "all" in n ? n.all : "any" in n ? n.any : null;
    if (!Array.isArray(list)) return "…";
    return `(${list.map(go).join("all" in n ? " and " : " or ")})`;
  };
  return go(tree);
}

/** A stored rule in words, whether or not the builder can show it as a form. */
export function ruleSentence(
  r: { condition: unknown; weight: number; severity: string },
  catalog: CatalogField[],
): string {
  const d = fromCondition(r.condition, catalog);
  return d
    ? describe(d, catalog, { weight: r.weight, severity: r.severity })
    : `If ${treeText(r.condition, catalog)}, add ${r.weight} risk points (${r.severity} severity).`;
}
