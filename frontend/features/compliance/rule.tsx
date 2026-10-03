"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { CodeBlock, KeyValues, Tag } from "@/components/ui/misc";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import { Empty, ErrorState, Loading, Notice } from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { ApiError, type Schemas } from "@/lib/api/client";
import {
  defaultLabel,
  formatDefault,
  formatParameterValue,
  formatRange,
  KIND_LABEL,
  MODE_LABEL,
  VERIFICATION,
  wordingHeadings,
} from "@/lib/compliance";
import { humanize } from "@/lib/format";
import {
  useComplianceConfig,
  useCompliancePack,
  useComplianceRule,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { ConfigHistory, ConfigureDialog, PendingChange } from "./configure";

type Pack = Schemas["CompliancePack"];
type Rule = Schemas["ComplianceRule"];

const list = (items: string[], fallback = "–") =>
  items.length ? items.join(", ") : fallback;

/** One rule of the library: its wording verbatim, its legal basis and how it runs. */
export function ComplianceRule({ id }: { id: string }) {
  const rule = useComplianceRule(id);
  const pack = useCompliancePack();
  const back = (
    <Link href="/compliance" className="hover:underline">
      Rules library
    </Link>
  );
  if (rule.isLoading || pack.isLoading) {
    return (
      <>
        <PageHeader title={id} eyebrow={back} />
        <Loading label="Loading the rule" />
      </>
    );
  }
  if (rule.isError || pack.isError || !rule.data || !pack.data) {
    const missing = rule.error instanceof ApiError && rule.error.status === 404;
    return (
      <>
        <PageHeader title={id} eyebrow={back} />
        {missing ? (
          <Panel>
            <Empty title="No such rule">
              The pack your workspace is on has no rule {id}.
            </Empty>
          </Panel>
        ) : (
          <ErrorState error={rule.error ?? pack.error} />
        )}
      </>
    );
  }
  return <RuleView rule={rule.data} pack={pack.data} back={back} />;
}

function RuleView({
  rule,
  pack,
  back,
}: {
  rule: Rule;
  pack: Pack;
  back: ReactNode;
}) {
  const headings = wordingHeadings(rule.kind);
  const second = rule.kind === "control" ? rule.action : rule.points;
  const label = (opts: Schemas["ComplianceOption"][], ids: string[]) =>
    ids.map((i) => opts.find((o) => o.id === i)?.label ?? i);
  const group =
    pack.groups.find((g) => g.id === rule.group)?.label ?? rule.group;
  const verification = VERIFICATION[rule.verification];
  const { can, environment } = useSession();
  const config = useComplianceConfig(rule.id);
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState<string>();
  const pending = config.data?.pending ?? null;
  const configurable = can("compliance:configure");
  return (
    <>
      <PageHeader
        title={rule.name}
        eyebrow={
          <>
            {back} / <span className="mono">{rule.id}</span>
          </>
        }
        actions={
          <>
            <Tag>{KIND_LABEL[rule.kind]}</Tag>
            <StatusPill tone={verification.tone}>
              {verification.label}
            </StatusPill>
            <StatusPill tone={rule.mode === "live" ? "success" : "neutral"}>
              {MODE_LABEL[rule.mode]} in {environment}
            </StatusPill>
          </>
        }
      />
      {message ? (
        <div className="mb-5">
          <Notice tone="ok">{message}</Notice>
        </div>
      ) : null}
      {pending ? (
        <div className="mb-5">
          <PendingChange rule={rule} change={pending} />
        </div>
      ) : null}
      {rule.caveat ? (
        <div className="mb-5">
          <Notice tone="warn">
            <span className="font-medium">Caveat. </span>
            {rule.caveat}
          </Notice>
        </div>
      ) : null}
      <div className="grid gap-5 lg:grid-cols-[3fr_2fr]">
        <div className="flex min-w-0 flex-col gap-5">
          <Panel>
            <PanelHeader title="The rule" legend="the library's wording" />
            <div className="flex flex-col gap-4 px-5 pb-5">
              <section>
                <h3 className="text-label text-text-muted">
                  {headings.description}
                </h3>
                <p className="mt-1 max-w-prose text-body">{rule.description}</p>
              </section>
              {headings.second && second ? (
                <section>
                  <h3 className="text-label text-text-muted">
                    {headings.second}
                  </h3>
                  <p className="mt-1 max-w-prose text-body">{second}</p>
                </section>
              ) : null}
            </div>
          </Panel>
          <Panel className="overflow-hidden">
            <PanelHeader
              title="Settings"
              legend={
                rule.config_version
                  ? `version ${rule.config_version} in ${environment}`
                  : `library defaults in ${environment}`
              }
              actions={
                configurable ? (
                  <Button
                    size="sm"
                    onClick={() => {
                      setMessage(undefined);
                      setEditing(true);
                    }}
                    disabled={!!pending}
                    title={
                      pending ? "A change is awaiting approval" : undefined
                    }
                  >
                    Configure
                  </Button>
                ) : null
              }
            />
            {rule.parameters.length ? (
              <Table aria-label="Settings">
                <thead>
                  <tr>
                    <Th>Setting</Th>
                    <Th>In use</Th>
                    <Th>Default</Th>
                    <Th>Allowed</Th>
                  </tr>
                </thead>
                <tbody>
                  {rule.parameters.map((p) => (
                    <Tr key={p.name}>
                      <Td className="py-2.5">{p.label}</Td>
                      <Td
                        className={
                          p.value === null
                            ? "text-body-sm text-status-warning"
                            : p.value !== p.default
                              ? "mono text-data font-medium whitespace-nowrap"
                              : "mono text-data whitespace-nowrap"
                        }
                      >
                        {p.value === null
                          ? "Not set"
                          : formatParameterValue(p, p.value)}
                      </Td>
                      <Td
                        className={
                          p.default === null
                            ? "text-body-sm text-status-warning"
                            : "mono text-data whitespace-nowrap"
                        }
                      >
                        {formatDefault(p)}
                      </Td>
                      <Td className="mono text-data whitespace-nowrap text-text-secondary">
                        {formatRange(p)}
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </Table>
            ) : (
              <p className="px-5 pb-5 text-body-sm text-text-secondary">
                This rule has no settings.
              </p>
            )}
          </Panel>
          <ConfigHistory rule={rule} onDone={setMessage} />
          {rule.engineering_logic ? (
            <Panel>
              <PanelHeader
                title="Engineering logic"
                legend="for engineers and administrators"
              />
              <div className="flex flex-col gap-3 px-5 pb-5">
                <CodeBlock
                  value={rule.engineering_logic}
                  label="Engineering logic"
                />
                {rule.definition ? (
                  <details>
                    <summary className="cursor-pointer text-body-sm text-text-secondary">
                      Machine definition the engine interprets
                    </summary>
                    <CodeBlock
                      value={rule.definition}
                      label="Rule definition"
                      className="mt-2"
                    />
                  </details>
                ) : null}
              </div>
            </Panel>
          ) : null}
        </div>
        <div className="flex min-w-0 flex-col gap-5">
          <Panel>
            <PanelHeader title="Legal basis" />
            <div className="px-5 pb-5">
              <KeyValues
                items={[
                  ["Regulators", list(rule.regulators)],
                  [
                    "In effect",
                    rule.effective.date
                      ? `${rule.effective.text} (${rule.effective.date})`
                      : rule.effective.text,
                  ],
                  ["Verification", verification.label],
                  [
                    "Sources",
                    <ul key="s" className="flex flex-col gap-1">
                      {rule.sources.map((id) => {
                        const s = pack.sources.find((x) => x.id === id);
                        const link = s?.links[0];
                        return (
                          <li key={id}>
                            <span className="mono mr-1.5 text-data text-text-muted">
                              {id}
                            </span>
                            {link ? (
                              <a
                                href={link.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-accent hover:underline"
                              >
                                {s.title}
                              </a>
                            ) : (
                              (s?.title ?? id)
                            )}
                          </li>
                        );
                      })}
                    </ul>,
                  ],
                ]}
              />
            </div>
          </Panel>
          <Panel>
            <PanelHeader title="How it runs" />
            <div className="flex flex-col gap-3 px-5 pb-5">
              <KeyValues
                items={[
                  ["Group", group],
                  ["By default", defaultLabel(rule.defaults)],
                  [`In ${environment}`, MODE_LABEL[rule.mode]],
                  [
                    "Applies to",
                    list(
                      label(pack.licence_types, rule.applies_to),
                      "Every institution",
                    ),
                  ],
                  ...(rule.requires.length
                    ? ([
                        [
                          "Needs",
                          list(label(pack.capabilities, rule.requires)),
                        ],
                      ] as [string, string][])
                    : []),
                  [
                    "For you",
                    rule.applicable === null
                      ? "Set your licence type to see"
                      : rule.applicable
                        ? "Applies"
                        : "Does not apply",
                  ],
                  [
                    "Checked on",
                    list(rule.events.map(humanize), "A schedule or workflow"),
                  ],
                  [
                    "Outcome",
                    list(
                      [
                        rule.outcome.decision &&
                          humanize(rule.outcome.decision),
                        rule.outcome.severity &&
                          `${humanize(rule.outcome.severity)} severity`,
                        ...rule.outcome.tasks.map(humanize),
                      ].filter((x): x is string => !!x),
                    ),
                  ],
                  ["Data it uses", list(rule.inputs.map(humanize))],
                ]}
              />
              <p className="text-body-sm text-text-muted">
                When any of this data is unavailable, the rule is reported as
                not evaluated, never as no risk.
              </p>
            </div>
          </Panel>
        </div>
      </div>
      {configurable && editing ? (
        <ConfigureDialog
          rule={rule}
          open={editing}
          onOpenChange={setEditing}
          onDone={setMessage}
        />
      ) : null}
    </>
  );
}
