import Link from "next/link";
import type { Route } from "next";
import type { ReactNode } from "react";

import { CodeBlock } from "@/components/ui/misc";
import { Notice } from "@/components/ui/states";

import { CodeTabs } from "./code-tabs";

/** Building blocks of a documentation page: one look for every guide. */

export function DocPage({
  title,
  lead,
  children,
}: {
  title: string;
  lead?: ReactNode;
  children: ReactNode;
}) {
  return (
    <article className="max-w-3xl">
      <h1 className="text-display font-bold tracking-tight">{title}</h1>
      {lead ? (
        <p className="mt-4 text-body-lg text-text-secondary">{lead}</p>
      ) : null}
      <div className="mt-10 flex flex-col gap-5">{children}</div>
    </article>
  );
}

const slug = (s: string) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

/** A section heading people can link to. */
export function H2({ children, id }: { children: string; id?: string }) {
  const anchor = id ?? slug(children);
  return (
    <h2
      id={anchor}
      className="group mt-8 scroll-mt-24 text-heading-1 font-semibold"
    >
      {children}
      <a
        href={`#${anchor}`}
        className="ml-2 text-text-muted opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
        aria-label={`Link to ${children}`}
      >
        #
      </a>
    </h2>
  );
}

export function H3({ children, id }: { children: string; id?: string }) {
  return (
    <h3
      id={id ?? slug(children)}
      className="mt-4 scroll-mt-24 text-heading-2 font-semibold"
    >
      {children}
    </h3>
  );
}

export function P({ children }: { children: ReactNode }) {
  return (
    <p className="text-body leading-relaxed text-text-secondary">{children}</p>
  );
}

/** Inline code: a field, a header, a value. */
export function C({ children }: { children: ReactNode }) {
  return (
    <code className="mono rounded-sm bg-bg-sunken px-1 py-0.5 text-data text-text-primary">
      {children}
    </code>
  );
}

export function UL({ children }: { children: ReactNode }) {
  return (
    <ul className="ml-5 flex list-disc flex-col gap-1.5 text-body leading-relaxed text-text-secondary">
      {children}
    </ul>
  );
}

export function OL({ children }: { children: ReactNode }) {
  return (
    <ol className="ml-5 flex list-decimal flex-col gap-1.5 text-body leading-relaxed text-text-secondary">
      {children}
    </ol>
  );
}

export function Code({ code, label }: { code: string; label: string }) {
  return <CodeBlock value={code} label={label} />;
}

export function Samples({
  label,
  samples,
}: {
  label: string;
  samples: { lang: string; code: string }[];
}) {
  return <CodeTabs label={label} samples={samples} />;
}

export function Callout({
  tone = "info",
  children,
}: {
  tone?: "info" | "warn" | "ok";
  children: ReactNode;
}) {
  return <Notice tone={tone}>{children}</Notice>;
}

/** A table of fields, values or codes: the first column is shown as code. */
export function Fields({
  label,
  head,
  rows,
}: {
  label: string;
  head: string[];
  rows: ReactNode[][];
}) {
  return (
    <div
      className="overflow-x-auto rounded-md border border-border-subtle"
      role="region"
      aria-label={label}
      tabIndex={0}
    >
      <table className="w-full border-collapse text-body-sm" aria-label={label}>
        <thead>
          <tr>
            {head.map((h) => (
              <th
                key={h}
                className="border-b border-border-subtle bg-bg-sunken px-3 py-2 text-left text-label font-medium text-text-muted"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-b border-border-subtle last:border-0">
              {r.map((cell, j) => (
                <td
                  key={j}
                  className={`px-3 py-2 align-top ${j === 0 ? "whitespace-nowrap" : "text-text-secondary"}`}
                >
                  {j === 0 && typeof cell === "string" ? <C>{cell}</C> : cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** A link to another page of the docs or the API reference. */
export function DocLink({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href as Route}
      className="font-medium text-accent underline-offset-2 hover:underline"
    >
      {children}
    </Link>
  );
}

/** Next page at the foot of a guide. */
export function Next({ href, label }: { href: string; label: string }) {
  return (
    <div className="mt-12 border-t border-border-subtle pt-6">
      <p className="text-label text-text-muted">Next</p>
      <DocLink href={href}>{label} →</DocLink>
    </div>
  );
}
