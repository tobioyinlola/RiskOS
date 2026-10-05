"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { KeyValues } from "@/components/ui/misc";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import {
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { CLOCK_KIND } from "@/lib/compliance";
import { formatTimestamp } from "@/lib/format";
import {
  useClockMatrix,
  useComplianceClocks,
  useComplianceGaps,
  useComplianceGlossary,
  useComplianceProfile,
  useComplianceChanges,
  useUserName,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { ProfileDialog, profileSummary } from "./profile";

type Pack = Schemas["CompliancePack"];

/** A rule ID linking to its page, in its country when given. */
export function RuleLink({
  id,
  jurisdiction,
}: {
  id: string;
  jurisdiction?: string;
}) {
  return (
    <Link
      href={
        (jurisdiction
          ? `/compliance/rules/${id}?jurisdiction=${jurisdiction}`
          : `/compliance/rules/${id}`) as "/compliance"
      }
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

function RuleLinks({
  ids,
  jurisdiction,
}: {
  ids: string[];
  jurisdiction?: string;
}) {
  if (ids.length === 0) return <span className="text-text-muted">–</span>;
  return (
    <span className="flex flex-wrap gap-x-2">
      {ids.map((id) => (
        <RuleLink key={id} id={id} jurisdiction={jurisdiction} />
      ))}
    </span>
  );
}

/**
 * A country's deadlines, each labelled by what it rests on: a legal clock, internal policy, a
 * design placeholder, or a legal deadline no readable source states (never shown as zero).
 */
export function ClocksTab({ jurisdiction }: { jurisdiction?: string }) {
  const q = useComplianceClocks(jurisdiction);
  const clocks = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Deadlines"
        legend="legal clocks, internal policy and what is not yet known"
      />
      <Table aria-label="Deadlines">
        <thead>
          <tr>
            <Th>Deadline</Th>
            <Th>What it is</Th>
            <Th>Duty</Th>
            <Th>Who</Th>
            <Th>Requirement</Th>
            <Th>Rules</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={6} rows={4} /> : null}
          {clocks.map((c) => (
            <Tr key={c.id}>
              <Td className="font-medium whitespace-nowrap">
                {c.hours === null && c.working_days === undefined
                  ? "Not found"
                  : c.duration}
                {c.authority ? (
                  <span className="block text-label font-normal text-text-muted">
                    to the {c.authority}
                  </span>
                ) : null}
              </Td>
              <Td className="whitespace-nowrap">
                <StatusPill tone={CLOCK_KIND[c.kind].tone}>
                  {CLOCK_KIND[c.kind].label}
                </StatusPill>
              </Td>
              <Td className="text-body-sm">{c.duty}</Td>
              <Td className="text-body-sm text-text-secondary">
                {c.applies_to}
              </Td>
              <Td className="py-2.5 text-body-sm text-text-secondary">
                {c.requirement}
              </Td>
              <Td>
                <RuleLinks ids={c.rules} jurisdiction={jurisdiction} />
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
    </Panel>
  );
}

const GAP_KIND: Record<
  NonNullable<Schemas["ComplianceGap"]["kind"]>,
  { label: string; tone: "warning" | "neutral" }
> = {
  conflict: { label: "Sources conflict", tone: "warning" },
  unverified: { label: "Not yet verified", tone: "neutral" },
};

/** A country's open questions: conflicts and unverified items, with how each is treated. */
export function GapsTab({ jurisdiction }: { jurisdiction?: string }) {
  const q = useComplianceGaps(jurisdiction);
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
          <PanelHeader
            title={g.title}
            legend={g.id}
            actions={
              g.kind ? (
                <StatusPill tone={GAP_KIND[g.kind].tone}>
                  {GAP_KIND[g.kind].label}
                </StatusPill>
              ) : null
            }
          />
          <div className="flex flex-col gap-3 px-5 pb-5">
            <p className="max-w-prose text-body text-text-secondary">
              {g.detail}
            </p>
            <GapVerification gap={g} jurisdiction={jurisdiction} />
            <KeyValues
              items={[
                ...(g.treatment
                  ? ([["Until it is settled", g.treatment]] as [
                      string,
                      string,
                    ][])
                  : []),
                [
                  "Rules",
                  <RuleLinks
                    key="r"
                    ids={g.rules}
                    jurisdiction={jurisdiction}
                  />,
                ],
                ["Sources", g.sources.join(", ") || "–"],
              ]}
            />
          </div>
        </Panel>
      ))}
    </div>
  );
}

/** The library's side-by-side comparison of Nigeria, Ghana and Rwanda, verbatim. */
export function CountriesSideBySide() {
  const q = useClockMatrix();
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Countries side by side"
        legend="each event is judged by the country that governs it, not the strictest of the three"
      />
      <div className="overflow-x-auto">
        <Table aria-label="Countries side by side">
          <thead>
            <tr>
              <Th>Item</Th>
              <Th>Nigeria</Th>
              <Th>Ghana</Th>
              <Th>Rwanda</Th>
              <Th>Working default</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={5} rows={5} /> : null}
            {(q.data?.data ?? []).map((r) => (
              <Tr key={r.item}>
                <Td className="font-medium">{r.item}</Td>
                <Td className="py-2.5 text-body-sm">{r.nigeria}</Td>
                <Td className="py-2.5 text-body-sm">{r.ghana}</Td>
                <Td className="py-2.5 text-body-sm">{r.rwanda}</Td>
                <Td className="py-2.5 text-body-sm text-text-secondary">
                  {r.working_default}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </div>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
    </Panel>
  );
}

/** Download a country's library (or every country's) as CSV or PDF. Audited. */
export function ExportLibrary({
  jurisdiction,
  name,
}: {
  jurisdiction?: string;
  name: string;
}) {
  const download = useAction(async (format: "csv" | "pdf") => {
    const res = await api().GET("/api/v1/compliance/export", {
      params: {
        query: { format, ...(jurisdiction ? { jurisdiction } : {}) },
      },
      parseAs: "blob",
    });
    const blob = unwrap(res) as unknown as Blob;
    const disposition = res.response.headers.get("Content-Disposition") ?? "";
    const file =
      /filename="([^"]+)"/.exec(disposition)?.[1] ?? `riskos-rules.${format}`;
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = file;
    a.click();
    URL.revokeObjectURL(url);
    return file;
  });
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-body-sm text-text-secondary">
        Download {name}&apos;s rules:
      </span>
      <Button
        size="sm"
        disabled={download.busy}
        onClick={() => void download.run("csv")}
      >
        Spreadsheet (CSV)
      </Button>
      <Button
        size="sm"
        disabled={download.busy}
        onClick={() => void download.run("pdf")}
      >
        Document (PDF)
      </Button>
      {download.error ? <Notice tone="error">{download.error}</Notice> : null}
    </div>
  );
}

export function GlossaryTab({ jurisdiction }: { jurisdiction?: string }) {
  const q = useComplianceGlossary(jurisdiction);
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

/** Who checked an open question against the original text, or a form to record it. */
function GapVerification({
  gap,
  jurisdiction,
}: {
  gap: Schemas["ComplianceGap"];
  jurisdiction?: string;
}) {
  const { can } = useSession();
  const userName = useUserName();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [link, setLink] = useState("");
  const verify = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/gaps/{gap_id}/verify", {
          params: {
            path: { gap_id: gap.id },
            query: jurisdiction ? { jurisdiction } : {},
          },
          body: link.trim() ? { note, link: link.trim() } : { note },
        }),
      ),
    { invalidate: ["compliance"], onSuccess: () => setOpen(false) },
  );
  const v = gap.verification;
  if (v)
    return (
      <Notice tone="ok">
        <span className="font-medium">Verified against the original text</span>{" "}
        by {userName(v.verified_by)} on {formatTimestamp(v.verified_at)}:{" "}
        {v.note}
        {v.link ? (
          <>
            {" "}
            <a
              href={v.link}
              className="underline"
              target="_blank"
              rel="noreferrer noopener"
            >
              Source
            </a>
          </>
        ) : null}
      </Notice>
    );
  if (!can("compliance:configure")) return null;
  if (!open)
    return (
      <div>
        <Button size="sm" onClick={() => setOpen(true)}>
          Mark verified
        </Button>
      </div>
    );
  return (
    <form
      className="flex flex-col gap-3 rounded-md border border-border-subtle p-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (note.trim()) void verify.run();
      }}
    >
      {verify.error ? <Notice tone="error">{verify.error}</Notice> : null}
      <Field
        label="What you checked"
        hint="The original text you read and what it says. This does not change any rule's settings."
      >
        {(f) => (
          <Textarea
            {...f}
            value={note}
            maxLength={1000}
            required
            onChange={(e) => setNote(e.target.value)}
          />
        )}
      </Field>
      <Field label="Link to the original (optional)">
        {(f) => (
          <Input
            {...f}
            type="url"
            value={link}
            maxLength={500}
            onChange={(e) => setLink(e.target.value)}
          />
        )}
      </Field>
      <div className="flex gap-2">
        <Button
          type="submit"
          size="sm"
          variant="primary"
          disabled={verify.busy || !note.trim()}
        >
          Record verification
        </Button>
        <Button type="button" size="sm" onClick={() => setOpen(false)}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
