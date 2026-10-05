import type { ReactNode } from "react";

import specJson from "@/lib/api/openapi.gen.json";

/**
 * The API reference, built from the OpenAPI contract the server is generated from
 * (lib/api/openapi.gen.json, written by `make generate`): every operation, parameter, field,
 * value and response, so the reference cannot drift from the API.
 */

interface Schema {
  type?: string;
  $ref?: string;
  description?: string;
  properties?: Record<string, Schema>;
  required?: string[];
  items?: Schema;
  enum?: unknown[];
  format?: string;
  pattern?: string;
  oneOf?: Schema[];
  anyOf?: Schema[];
  allOf?: Schema[];
  additionalProperties?: boolean | Schema;
  nullable?: boolean;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  minItems?: number;
  maxItems?: number;
  default?: unknown;
}
interface Parameter {
  $ref?: string;
  name: string;
  in: string;
  required?: boolean;
  description?: string;
  schema?: Schema;
}
interface Response {
  $ref?: string;
  description?: string;
  content?: Record<string, { schema?: Schema }>;
}
interface Operation {
  operationId: string;
  summary?: string;
  description?: string;
  tags?: string[];
  parameters?: Parameter[];
  requestBody?: {
    required?: boolean;
    content?: Record<string, { schema?: Schema }>;
  };
  responses?: Record<string, Response>;
  security?: unknown[];
}
interface Spec {
  info: { title: string; version: string; description?: string };
  tags: { name: string; description?: string }[];
  paths: Record<string, Record<string, Operation | Parameter[]>>;
  components: {
    schemas: Record<string, Schema>;
    parameters: Record<string, Parameter>;
    responses: Record<string, Response>;
  };
}

const spec = specJson as unknown as Spec;
const METHODS = ["get", "post", "put", "patch", "delete"] as const;

export interface OperationRef {
  method: string;
  path: string;
  op: Operation;
}

/** Tags in the contract's order, with their operations. */
export function referenceTags(): {
  name: string;
  description: string;
  ops: OperationRef[];
}[] {
  return spec.tags.map((t) => ({
    name: t.name,
    description: t.description ?? "",
    ops: operationsOf(t.name),
  }));
}

const TAG_TITLES: Record<string, string> = {
  auth: "Credentials and API keys",
  audit: "Audit log",
  onboarding: "Onboarding",
  session: "Console sessions",
};

/** A tag's page title: "rules" reads "Rules". */
export function tagTitle(tag: string): string {
  return TAG_TITLES[tag] ?? tag.charAt(0).toUpperCase() + tag.slice(1);
}

function operationsOf(tag: string): OperationRef[] {
  const out: OperationRef[] = [];
  for (const [path, item] of Object.entries(spec.paths)) {
    const shared = (item.parameters as Parameter[] | undefined) ?? [];
    for (const m of METHODS) {
      const op = item[m] as Operation | undefined;
      if (op?.tags?.includes(tag))
        out.push({
          method: m,
          path,
          op: { ...op, parameters: [...shared, ...(op.parameters ?? [])] },
        });
    }
  }
  return out;
}

export const specVersion = () => spec.info.version;
export const specDescription = () => spec.info.description ?? "";

const refName = (ref: string) => ref.split("/").at(-1)!;
function schemaOf(s: Schema | undefined): Schema | undefined {
  return s?.$ref ? spec.components.schemas[refName(s.$ref)] : s;
}

/** The small part of Markdown the contract uses: paragraphs, lists, code, bold and links. */
export function Md({ text }: { text?: string }) {
  if (!text) return null;
  const blocks = text.trim().split(/\n\s*\n/);
  return (
    <>
      {blocks.map((b, i) => {
        const lines = b.split("\n");
        if (lines.every((l) => /^\s*[-*] /.test(l) || /^\s{2,}\S/.test(l))) {
          const items: string[] = [];
          for (const l of lines) {
            if (/^\s*[-*] /.test(l)) items.push(l.replace(/^\s*[-*] /, ""));
            else items[items.length - 1] += ` ${l.trim()}`;
          }
          return (
            <ul
              key={i}
              className="my-2 ml-5 list-disc text-body-sm text-text-secondary"
            >
              {items.map((it, j) => (
                <li key={j}>
                  <Inline text={it} />
                </li>
              ))}
            </ul>
          );
        }
        return (
          <p
            key={i}
            className="my-2 text-body-sm leading-relaxed text-text-secondary"
          >
            <Inline text={lines.join(" ")} />
          </p>
        );
      })}
    </>
  );
}

function Inline({ text }: { text: string }) {
  const parts: ReactNode[] = [];
  const re = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)]+)\)/g;
  let last = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    if (m[1] !== undefined)
      parts.push(
        <code
          key={m.index}
          className="mono rounded-sm bg-bg-sunken px-1 text-data text-text-primary"
        >
          {m[1]}
        </code>,
      );
    else if (m[2] !== undefined)
      parts.push(
        <strong key={m.index} className="font-semibold text-text-primary">
          {m[2]}
        </strong>,
      );
    else if (/^https:\/\//.test(m[4]!))
      parts.push(
        <a
          key={m.index}
          href={m[4]}
          className="text-accent underline"
          rel="noopener noreferrer"
        >
          {m[3]}
        </a>,
      );
    else parts.push(m[3]);
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}

/** A field's type in words: "string (date-time)", "one of: a, b", "array of Signal"... */
function typeText(s: Schema): string {
  if (s.$ref) return refName(s.$ref);
  if (s.oneOf) return `one of: ${s.oneOf.map(typeText).join(", ")}`;
  if (s.allOf) return s.allOf.map(typeText).join(" & ");
  if (s.type === "array")
    return `array of ${s.items ? typeText(s.items) : "values"}`;
  const t = s.type ?? "object";
  return s.format ? `${t} (${s.format})` : t;
}

function limits(s: Schema): string[] {
  const out: string[] = [];
  if (s.enum)
    out.push(`one of ${s.enum.map((e) => `\`${String(e)}\``).join(", ")}`);
  if (s.pattern) out.push(`pattern \`${s.pattern}\``);
  if (s.minimum !== undefined) out.push(`minimum ${s.minimum}`);
  if (s.maximum !== undefined) out.push(`maximum ${s.maximum}`);
  if (s.minLength !== undefined) out.push(`at least ${s.minLength} characters`);
  if (s.maxLength !== undefined) out.push(`at most ${s.maxLength} characters`);
  if (s.minItems !== undefined) out.push(`at least ${s.minItems} items`);
  if (s.maxItems !== undefined) out.push(`at most ${s.maxItems} items`);
  if (s.default !== undefined)
    out.push(`default \`${JSON.stringify(s.default)}\``);
  if (s.nullable) out.push("may be null");
  return out;
}

/** An object's fields, with nested objects behind a disclosure, to a bounded depth. */
export function SchemaView({
  schema,
  depth = 0,
  seen = [],
}: {
  schema: Schema | undefined;
  depth?: number;
  seen?: string[];
}) {
  const s = schemaOf(schema);
  if (!s) return null;
  const name = schema?.$ref ? refName(schema.$ref) : undefined;
  if (name && seen.includes(name))
    return <p className="text-label text-text-muted">{name} (see above)</p>;
  const now = name ? [...seen, name] : seen;
  if (s.oneOf)
    return (
      <div className="flex flex-col gap-2">
        <p className="text-body-sm text-text-secondary">One of:</p>
        {s.oneOf.map((v, i) => (
          <details
            key={i}
            className="rounded-md border border-border-subtle px-3 py-2"
          >
            <summary className="cursor-pointer text-body-sm font-medium">
              {typeText(v)}
            </summary>
            <Md text={schemaOf(v)?.description} />
            <SchemaView schema={v} depth={depth + 1} seen={now} />
          </details>
        ))}
      </div>
    );
  if (s.type === "array" && s.items) {
    return <SchemaView schema={s.items} depth={depth} seen={now} />;
  }
  const props = Object.entries(s.properties ?? {});
  if (!props.length) {
    if (s.additionalProperties && typeof s.additionalProperties === "object")
      return (
        <p className="text-body-sm text-text-secondary">
          A map of names to {typeText(s.additionalProperties)}.
        </p>
      );
    return null;
  }
  return (
    <div
      className="overflow-x-auto rounded-md border border-border-subtle"
      role="region"
      tabIndex={0}
      aria-label={`${name ?? "Object"} fields`}
    >
      <table className="w-full border-collapse text-body-sm">
        <thead>
          <tr>
            <th className="border-b border-border-subtle bg-bg-sunken px-3 py-2 text-left text-label font-medium text-text-muted">
              Field
            </th>
            <th className="border-b border-border-subtle bg-bg-sunken px-3 py-2 text-left text-label font-medium text-text-muted">
              Type and meaning
            </th>
          </tr>
        </thead>
        <tbody>
          {props.map(([key, raw]) => {
            const f = schemaOf(raw) ?? raw;
            const required = s.required?.includes(key);
            const inner = raw.type === "array" ? raw.items : raw;
            const nested =
              depth < 3 &&
              inner &&
              (inner.$ref || inner.properties || inner.oneOf) &&
              Object.keys(schemaOf(inner)?.properties ?? {}).length +
                (schemaOf(inner)?.oneOf?.length ?? 0) >
                0;
            return (
              <tr
                key={key}
                className="border-b border-border-subtle align-top last:border-0"
              >
                <td className="px-3 py-2 whitespace-nowrap">
                  <code className="mono text-data text-text-primary">
                    {key}
                  </code>
                  {required ? (
                    <span className="ml-1 text-label text-status-danger">
                      required
                    </span>
                  ) : null}
                </td>
                <td className="px-3 py-2">
                  <p className="mono text-data text-text-muted">
                    {typeText(raw)}
                  </p>
                  <Md
                    text={
                      raw.description ?? (raw.$ref ? undefined : f.description)
                    }
                  />
                  {limits(f).length ? (
                    <p className="text-label text-text-muted">
                      <Inline text={limits(f).join(" · ")} />
                    </p>
                  ) : null}
                  {nested ? (
                    <details className="mt-1">
                      <summary className="cursor-pointer text-label text-text-secondary">
                        Fields of {typeText(inner)}
                      </summary>
                      <div className="mt-2">
                        <SchemaView
                          schema={inner}
                          depth={depth + 1}
                          seen={now}
                        />
                      </div>
                    </details>
                  ) : null}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

const METHOD_TONE: Record<string, string> = {
  get: "bg-status-info-bg text-status-info",
  post: "bg-status-success-bg text-status-success",
  put: "bg-status-warning-bg text-status-warning",
  patch: "bg-status-warning-bg text-status-warning",
  delete: "bg-status-danger-bg text-status-danger",
};

export function MethodBadge({ method }: { method: string }) {
  return (
    <span
      className={`mono inline-block min-w-14 rounded-sm px-1.5 py-0.5 text-center text-label font-semibold uppercase ${METHOD_TONE[method] ?? ""}`}
    >
      {method}
    </span>
  );
}

function resolveParam(p: Parameter): Parameter {
  return p.$ref ? spec.components.parameters[refName(p.$ref)]! : p;
}
function resolveResponse(r: Response): Response {
  return r.$ref ? spec.components.responses[refName(r.$ref)]! : r;
}

/** One operation: what it does, who may call it, what it takes and what it answers. */
export function OperationView({ method, path, op }: OperationRef) {
  const params = (op.parameters ?? []).map(resolveParam);
  const body = op.requestBody?.content?.["application/json"]?.schema;
  return (
    <section
      id={op.operationId}
      className="scroll-mt-24 border-t border-border-subtle pt-8"
    >
      <h2 className="text-heading-2 font-semibold">
        {op.summary ?? op.operationId}
      </h2>
      <p className="mt-2 flex flex-wrap items-center gap-2">
        <MethodBadge method={method} />
        <code className="mono text-data break-all text-text-primary">
          {path}
        </code>
      </p>
      <div className="mt-3">
        <Md text={op.description} />
      </div>
      {params.length ? (
        <div className="mt-4">
          <h3 className="mb-2 text-body-sm font-semibold">Parameters</h3>
          <div
            className="overflow-x-auto rounded-md border border-border-subtle"
            role="region"
            tabIndex={0}
            aria-label={`${op.operationId} parameters`}
          >
            <table className="w-full border-collapse text-body-sm">
              <tbody>
                {params.map((p) => {
                  const ps = p.schema
                    ? (schemaOf(p.schema) ?? p.schema)
                    : undefined;
                  return (
                    <tr
                      key={`${p.in}-${p.name}`}
                      className="border-b border-border-subtle align-top last:border-0"
                    >
                      <td className="px-3 py-2 whitespace-nowrap">
                        <code className="mono text-data">{p.name}</code>
                        <span className="ml-2 text-label text-text-muted">
                          {p.in}
                        </span>
                        {p.required ? (
                          <span className="ml-1 text-label text-status-danger">
                            required
                          </span>
                        ) : null}
                      </td>
                      <td className="px-3 py-2">
                        {p.schema ? (
                          <p className="mono text-data text-text-muted">
                            {typeText(p.schema)}
                          </p>
                        ) : null}
                        <Md text={p.description} />
                        {ps && limits(ps).length ? (
                          <p className="text-label text-text-muted">
                            <Inline text={limits(ps).join(" · ")} />
                          </p>
                        ) : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
      {body ? (
        <div className="mt-4">
          <h3 className="mb-2 text-body-sm font-semibold">
            Request body{op.requestBody?.required ? "" : " (optional)"}:{" "}
            {typeText(body)}
          </h3>
          <SchemaView schema={body} />
        </div>
      ) : null}
      <div className="mt-4">
        <h3 className="mb-2 text-body-sm font-semibold">Responses</h3>
        <ul className="flex flex-col gap-2">
          {Object.entries(op.responses ?? {}).map(([status, raw]) => {
            const r = resolveResponse(raw);
            const schema = r.content?.["application/json"]?.schema;
            const ok = status.startsWith("2");
            return (
              <li key={status} className="text-body-sm">
                <span
                  className={`mono mr-2 font-semibold ${ok ? "text-status-success" : "text-text-secondary"}`}
                >
                  {status === "default" ? "other" : status}
                </span>
                <span className="text-text-secondary">
                  <Inline text={(r.description ?? "").split("\n")[0] ?? ""} />
                </span>
                {ok && schema ? (
                  <details className="mt-1">
                    <summary className="cursor-pointer text-label text-text-secondary">
                      Response fields: {typeText(schema)}
                    </summary>
                    <div className="mt-2">
                      <SchemaView schema={schema} />
                    </div>
                  </details>
                ) : null}
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}
