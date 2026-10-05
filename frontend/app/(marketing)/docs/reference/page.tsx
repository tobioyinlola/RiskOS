import type { Metadata } from "next";

import {
  Md,
  MethodBadge,
  referenceTags,
  specVersion,
  tagTitle,
} from "@/features/docs/reference";
import { C, DocLink, DocPage, P } from "@/features/docs/ui";

export const metadata: Metadata = { title: "API reference" };

export default function Page() {
  const tags = referenceTags().filter((t) => t.ops.length);
  return (
    <DocPage
      title="API reference"
      lead={`Every endpoint of the RiskOS API, version ${specVersion()}, generated from the same contract the server is built from: it cannot drift from what the API does.`}
    >
      <P>
        Requests and responses are JSON. Authenticate with{" "}
        <C>Authorization: Bearer</C> and an API key, except for the console
        session group, which uses cookies. Errors follow{" "}
        <DocLink href="/docs/errors">the error contract</DocLink>.
      </P>
      {tags.map((t) => (
        <section key={t.name} className="mt-8">
          <h2 className="text-heading-2 font-semibold">
            <DocLink href={`/docs/reference/${t.name}`}>
              {tagTitle(t.name)}
            </DocLink>
          </h2>
          <Md text={t.description} />
          <ul className="mt-2 flex flex-col gap-1.5">
            {t.ops.map(({ method, path, op }) => (
              <li
                key={op.operationId}
                className="flex flex-wrap items-center gap-2 text-body-sm"
              >
                <MethodBadge method={method} />
                <DocLink href={`/docs/reference/${t.name}#${op.operationId}`}>
                  <span className="mono text-data">{path}</span>
                </DocLink>
                <span className="text-text-secondary">{op.summary}</span>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </DocPage>
  );
}
