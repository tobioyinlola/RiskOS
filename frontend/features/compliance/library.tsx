"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { KeyValues } from "@/components/ui/misc";
import { Panel, PanelHeader } from "@/components/ui/panel";
import {
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import {
  useComplianceClocks,
  useComplianceGaps,
  useComplianceGlossary,
  useComplianceProfile,
  useComplianceChanges,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { ProfileDialog, profileSummary } from "./profile";

type Pack = Schemas["CompliancePack"];

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

/** The workspace's licence type and capabilities, which decide what applies. */
export function ProfileBar({ pack }: { pack: Pack }) {
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
      {p && !p.confirmed ? (
        <Notice tone="warn">
          <span className="flex flex-wrap items-center gap-3">
            <span>
              {writable
                ? "Confirm what kind of institution you are, so the right rules apply. Until then RiskOS treats you as “Other”."
                : "A compliance officer or administrator should confirm the institution type, so the right rules apply."}
            </span>
            {writable ? (
              <Button size="sm" variant="primary" onClick={() => setOpen(true)}>
                Confirm institution type
              </Button>
            ) : null}
          </span>
        </Notice>
      ) : p ? (
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
      ) : null}
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

export function ClocksTab() {
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

export function GapsTab() {
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

export function GlossaryTab() {
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

export function SourcesTab({ pack }: { pack: Pack }) {
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

/** Changes awaiting a second person in this environment. */
export function PendingApprovals() {
  const { environment } = useSession();
  const q = useComplianceChanges("pending");
  const pending = q.data?.data ?? [];
  if (!pending.length) return null;
  return (
    <div className="mb-5">
      <Notice tone="warn">
        <span className="font-medium">
          {pending.length === 1
            ? "1 change awaits approval"
            : `${pending.length} changes await approval`}{" "}
          in {environment}:{" "}
        </span>
        <span className="inline-flex flex-wrap gap-x-2">
          {pending.map((c) => (
            <RuleLink key={c.id} id={c.rule} />
          ))}
        </span>
      </Notice>
    </div>
  );
}

/** A newer pack version is loaded: saving the profile adopts it. */
export function NewerVersion({ pack }: { pack: Pack }) {
  const { can } = useSession();
  const profile = useComplianceProfile();
  const p = profile.data;
  const adopt = useAction(
    async () =>
      unwrap(
        await api().PUT("/api/v1/compliance/profile", {
          body: {
            jurisdiction: pack.jurisdiction,
            licence_type: p!.licence_type,
            capabilities: p!.capabilities,
          },
        }),
      ),
    { invalidate: ["compliance"] },
  );
  if (!p || pack.latest_version === pack.version) return null;
  return (
    <div className="mb-5">
      <Notice tone="info">
        <span className="flex flex-wrap items-center gap-3">
          <span>
            Version {pack.latest_version} of the {pack.name} pack is available
            (you are on {pack.version}). Your rule settings carry over.
          </span>
          {can("compliance:write") ? (
            <Button
              size="sm"
              variant="primary"
              disabled={adopt.busy}
              onClick={() => void adopt.run()}
            >
              Adopt {pack.latest_version}
            </Button>
          ) : null}
        </span>
      </Notice>
    </div>
  );
}
