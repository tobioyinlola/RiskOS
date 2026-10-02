"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/field";
import {
  KeyValues,
  Status,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Tag,
} from "@/components/ui/misc";
import { Metric, PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { ApiError, type Schemas } from "@/lib/api/client";
import {
  defaultLabel,
  KIND_LABEL,
  matchesFilter,
  NO_FILTER,
  VERIFICATION,
  type RuleFilter,
} from "@/lib/compliance";
import { formatCount } from "@/lib/format";
import {
  useComplianceClocks,
  useComplianceGaps,
  useComplianceGlossary,
  useCompliancePack,
  useComplianceProfile,
  useComplianceRules,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { ProfileDialog, profileSummary } from "./profile";

type Pack = Schemas["CompliancePack"];
type Rule = Schemas["ComplianceRule"];

/** A rule ID linking to its page. */
export function RuleLink({ id }: { id: string }) {
  return (
    <Link
      href={`/compliance/rules/${id}`}
      className="mono text-data text-accent hover:underline"
    >
      {id}
    </Link>
  );
}

/** Compliance → Rules library: the jurisdiction pack the workspace is on. */
export function ComplianceLibrary() {
  const pack = useCompliancePack();
  if (pack.isLoading) {
    return (
      <>
        <PageHeader title="Rules library" />
        <Loading label="Loading the rules library" />
      </>
    );
  }
  if (pack.isError || !pack.data) {
    const none = pack.error instanceof ApiError && pack.error.status === 404;
    return (
      <>
        <PageHeader title="Rules library" />
        {none ? (
          <Panel>
            <Empty title="No rule pack is loaded">
              Rule packs are loaded when RiskOS is deployed. Ask your RiskOS
              contact which jurisdictions are available.
            </Empty>
          </Panel>
        ) : (
          <ErrorState error={pack.error} />
        )}
      </>
    );
  }
  return <Library pack={pack.data} />;
}

function Library({ pack }: { pack: Pack }) {
  const gaps = useComplianceGaps();
  return (
    <>
      <PageHeader
        title="Rules library"
        eyebrow={`${pack.name} · version ${pack.version}`}
        description="Every rule in the pack, in the library's own words, with its legal source, when it took effect and how far its facts are confirmed. Where sources conflict, it says so."
      />
      <ProfileBar pack={pack} />
      <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Metric
          label="Regulatory controls"
          value={String(pack.counts.controls)}
        />
        <Metric
          label="Risk-scoring rules"
          value={String(pack.counts.signals)}
        />
        <Metric
          label="Governance presets"
          value={String(pack.counts.governance)}
        />
        <Metric
          label="Open questions"
          value={gaps.data ? String(gaps.data.data.length) : "–"}
        />
      </div>
      <Tabs defaultValue="rules">
        <TabList aria-label="Library sections" className="mb-4">
          <Tab value="rules">Rules</Tab>
          <Tab value="clocks">Deadlines</Tab>
          <Tab value="gaps">Open questions</Tab>
          <Tab value="glossary">Glossary</Tab>
          <Tab value="sources">Sources</Tab>
        </TabList>
        <TabPanel value="rules">
          <RulesTab pack={pack} />
        </TabPanel>
        <TabPanel value="clocks">
          <ClocksTab />
        </TabPanel>
        <TabPanel value="gaps">
          <GapsTab />
        </TabPanel>
        <TabPanel value="glossary">
          <GlossaryTab />
        </TabPanel>
        <TabPanel value="sources">
          <SourcesTab pack={pack} />
        </TabPanel>
      </Tabs>
    </>
  );
}

/** The workspace's licence type and capabilities, which decide what applies. */
function ProfileBar({ pack }: { pack: Pack }) {
  const { can } = useSession();
  const profile = useComplianceProfile();
  const [open, setOpen] = useState(false);
  const writable = can("compliance:write");
  if (profile.isLoading) return null;
  if (profile.isError)
    return <ErrorState error={profile.error} className="mb-5" />;
  const p = profile.data;
  return (
    <div className="mb-5">
      {p ? (
        <Panel className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
          <div>
            <p className="text-label text-text-muted">Your institution</p>
            <p className="text-body">{profileSummary(pack, p)}</p>
          </div>
          {writable ? (
            <Button size="sm" onClick={() => setOpen(true)}>
              Change
            </Button>
          ) : null}
        </Panel>
      ) : (
        <Notice tone="info">
          <span className="flex flex-wrap items-center gap-3">
            <span>
              {writable
                ? "Tell RiskOS what kind of institution you are to see which rules apply to you."
                : "An administrator or compliance officer sets the institution's licence type, which decides which rules apply."}
            </span>
            {writable ? (
              <Button size="sm" variant="primary" onClick={() => setOpen(true)}>
                Set licence type
              </Button>
            ) : null}
          </span>
        </Notice>
      )}
      {writable ? (
        <ProfileDialog
          open={open}
          onOpenChange={setOpen}
          pack={pack}
          profile={p ?? null}
        />
      ) : null}
    </div>
  );
}

function AppliesCell({ applicable }: { applicable: boolean | null }) {
  if (applicable === null) return <span className="text-text-muted">–</span>;
  return applicable ? (
    <Status tone="ok">Applies</Status>
  ) : (
    <Status tone="off">Not for you</Status>
  );
}

function RulesTab({ pack }: { pack: Pack }) {
  const q = useComplianceRules();
  const [f, setF] = useState<RuleFilter>(NO_FILTER);
  const all = useMemo(() => q.data?.data ?? [], [q.data]);
  const rules = useMemo(() => all.filter((r) => matchesFilter(r, f)), [all, f]);
  const regulators = useMemo(
    () => [...new Set(all.flatMap((r) => r.regulators))].sort(),
    [all],
  );
  const hasProfile = all.some((r) => r.applicable !== null);
  const groupLabel = (id: string) =>
    pack.groups.find((g) => g.id === id)?.label ?? id;
  const set = <K extends keyof RuleFilter>(k: K, v: RuleFilter[K]) =>
    setF((cur) => ({ ...cur, [k]: v }));
  const control = "h-control-sm text-body-sm";
  return (
    <>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          type="search"
          aria-label="Search rules"
          placeholder="Search by ID or words"
          className={`${control} w-60`}
          value={f.query}
          onChange={(e) => set("query", e.target.value)}
        />
        <Select
          aria-label="Type"
          className={`${control} w-48`}
          value={f.kind}
          onChange={(e) => set("kind", e.target.value as RuleFilter["kind"])}
        >
          <option value="">All types</option>
          {(Object.keys(KIND_LABEL) as Rule["kind"][]).map((k) => (
            <option key={k} value={k}>
              {KIND_LABEL[k]}
            </option>
          ))}
        </Select>
        <Select
          aria-label="Group"
          className={`${control} w-52`}
          value={f.group}
          onChange={(e) => set("group", e.target.value)}
        >
          <option value="">All groups</option>
          {pack.groups.map((g) => (
            <option key={g.id} value={g.id}>
              {g.label}
            </option>
          ))}
        </Select>
        <Select
          aria-label="Regulator"
          className={`${control} w-44`}
          value={f.regulator}
          onChange={(e) => set("regulator", e.target.value)}
        >
          <option value="">All regulators</option>
          {regulators.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </Select>
        <Select
          aria-label="Verification"
          className={`${control} w-48`}
          value={f.verification}
          onChange={(e) =>
            set("verification", e.target.value as RuleFilter["verification"])
          }
        >
          <option value="">Any verification</option>
          {(Object.keys(VERIFICATION) as Rule["verification"][]).map((v) => (
            <option key={v} value={v}>
              {VERIFICATION[v].label}
            </option>
          ))}
        </Select>
        {hasProfile ? (
          <label className="inline-flex items-center gap-2 text-body-sm text-text-secondary">
            <input
              type="checkbox"
              className="size-4 accent-ink"
              checked={f.applicableOnly}
              onChange={(e) => set("applicableOnly", e.target.checked)}
            />
            Only rules that apply to us
          </label>
        ) : null}
        <span
          className="ml-auto text-label text-text-muted"
          role="status"
          aria-live="polite"
        >
          {q.data
            ? `${formatCount(rules.length)} of ${formatCount(all.length)} rules`
            : ""}
        </span>
      </div>
      <Panel className="overflow-hidden">
        <Table aria-label="Rules in the pack">
          <thead>
            <tr>
              <Th>Rule</Th>
              <Th>Type</Th>
              <Th>Regulators</Th>
              <Th>Verification</Th>
              <Th>Default</Th>
              <Th>Applies</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {rules.map((r) => (
              <Tr key={r.id}>
                <Td className="max-w-xl py-2.5">
                  <div className="flex items-baseline gap-2">
                    <RuleLink id={r.id} />
                    <span className="font-medium text-text-primary">
                      {r.name}
                    </span>
                  </div>
                  <p className="line-clamp-2 text-body-sm text-text-secondary">
                    {r.description}
                  </p>
                  <p className="text-label text-text-muted">
                    {groupLabel(r.group)}
                  </p>
                </Td>
                <Td>
                  <Tag>{KIND_LABEL[r.kind]}</Tag>
                </Td>
                <Td className="text-body-sm text-text-secondary">
                  {r.regulators.join(", ")}
                </Td>
                <Td>
                  <StatusPill tone={VERIFICATION[r.verification].tone}>
                    {VERIFICATION[r.verification].label}
                  </StatusPill>
                </Td>
                <Td className="text-body-sm">{defaultLabel(r.defaults)}</Td>
                <Td>
                  <AppliesCell applicable={r.applicable} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rules.length === 0 ? (
          <Empty
            title="No rules match"
            action={
              <Button size="sm" onClick={() => setF(NO_FILTER)}>
                Clear filters
              </Button>
            }
          />
        ) : null}
      </Panel>
    </>
  );
}

function RuleLinks({ ids }: { ids: string[] }) {
  if (ids.length === 0) return <span className="text-text-muted">–</span>;
  return (
    <span className="flex flex-wrap gap-x-2">
      {ids.map((id) => (
        <RuleLink key={id} id={id} />
      ))}
    </span>
  );
}

function ClocksTab() {
  const q = useComplianceClocks();
  const clocks = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Legal deadlines"
        legend="the timers reporting workflows run against"
      />
      <Table aria-label="Legal deadlines">
        <thead>
          <tr>
            <Th>Deadline</Th>
            <Th>Duty</Th>
            <Th>Who</Th>
            <Th>Requirement</Th>
            <Th>Rules</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={5} rows={4} /> : null}
          {clocks.map((c) => (
            <Tr key={c.id}>
              <Td className="font-medium whitespace-nowrap">{c.duration}</Td>
              <Td className="text-body-sm">{c.duty}</Td>
              <Td className="text-body-sm text-text-secondary">
                {c.applies_to}
              </Td>
              <Td className="py-2.5 text-body-sm text-text-secondary">
                {c.requirement}
              </Td>
              <Td>
                <RuleLinks ids={c.rules} />
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
    </Panel>
  );
}

function GapsTab() {
  const q = useComplianceGaps();
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  return (
    <div className="flex flex-col gap-4">
      <Notice tone="warn">
        These questions are open: the sources disagree or have not been
        confirmed. RiskOS shows them rather than guessing. Settle each with your
        compliance officer or counsel.
      </Notice>
      {(q.data?.data ?? []).map((g) => (
        <Panel key={g.id}>
          <PanelHeader title={g.title} legend={g.id} />
          <div className="flex flex-col gap-3 px-5 pb-5">
            <p className="max-w-prose text-body text-text-secondary">
              {g.detail}
            </p>
            <KeyValues
              items={[
                ["Rules", <RuleLinks key="r" ids={g.rules} />],
                ["Sources", g.sources.join(", ") || "–"],
              ]}
            />
          </div>
        </Panel>
      ))}
    </div>
  );
}

function GlossaryTab() {
  const q = useComplianceGlossary();
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  return (
    <Panel>
      <dl className="divide-y divide-border-subtle">
        {(q.data?.data ?? []).map((t) => (
          <div
            key={t.term}
            className="grid gap-1 px-5 py-3 sm:grid-cols-[200px_1fr]"
          >
            <dt className="font-medium">
              {t.term}
              {t.expansion && t.expansion !== t.term ? (
                <span className="block text-label font-normal text-text-muted">
                  {t.expansion}
                </span>
              ) : null}
            </dt>
            <dd className="text-body-sm text-text-secondary">{t.definition}</dd>
          </div>
        ))}
      </dl>
    </Panel>
  );
}

function SourcesTab({ pack }: { pack: Pack }) {
  return (
    <Panel>
      <ol className="divide-y divide-border-subtle">
        {pack.sources.map((s) => (
          <li key={s.id} id={`source-${s.id}`} className="px-5 py-3">
            <p className="text-body">
              <span className="mono mr-2 text-data text-text-muted">
                {s.id}
              </span>
              {s.title}
            </p>
            {s.links.length ? (
              <ul className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
                {s.links.map((l) => (
                  <li key={l.url}>
                    <a
                      href={l.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-body-sm text-accent hover:underline"
                    >
                      {l.title}
                    </a>
                  </li>
                ))}
              </ul>
            ) : null}
          </li>
        ))}
      </ol>
    </Panel>
  );
}
