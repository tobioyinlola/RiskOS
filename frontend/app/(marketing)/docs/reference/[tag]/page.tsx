import type { Metadata } from "next";
import { notFound } from "next/navigation";

import {
  Md,
  OperationView,
  referenceTags,
  tagTitle,
} from "@/features/docs/reference";
import { DocLink, DocPage } from "@/features/docs/ui";

export const dynamicParams = false;

export function generateStaticParams() {
  return referenceTags()
    .filter((t) => t.ops.length)
    .map((t) => ({ tag: t.name }));
}

export async function generateMetadata({
  params,
}: PageProps<"/docs/reference/[tag]">): Promise<Metadata> {
  const { tag } = await params;
  return { title: `${tagTitle(tag)}: API reference` };
}

export default async function Page({
  params,
}: PageProps<"/docs/reference/[tag]">) {
  const { tag } = await params;
  const group = referenceTags().find((t) => t.name === tag);
  if (!group?.ops.length) notFound();
  return (
    <DocPage title={tagTitle(group.name)}>
      <Md text={group.description} />
      <nav aria-label="Endpoints" className="mt-4 mb-8">
        <ul className="flex flex-col gap-1 text-body-sm">
          {group.ops.map(({ method, path, op }) => (
            <li key={op.operationId}>
              <DocLink href={`#${op.operationId}`}>
                {op.summary ?? op.operationId}
              </DocLink>
              <span className="mono ml-2 text-data text-text-muted">
                {method.toUpperCase()} {path}
              </span>
            </li>
          ))}
        </ul>
      </nav>
      <div className="flex flex-col gap-10">
        {group.ops.map((o) => (
          <OperationView key={o.op.operationId} {...o} />
        ))}
      </div>
      <p className="mt-12 border-t border-border-subtle pt-6 text-body-sm">
        <DocLink href="/docs/reference">← All endpoints</DocLink>
      </p>
    </DocPage>
  );
}
