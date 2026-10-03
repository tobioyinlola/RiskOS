"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Alert, Code, Investigate, Report } from "@/components/ui/icons";
import { Tab, TabList, TabPanel, Tabs } from "@/components/ui/misc";
import { OutlinePill, StatusPill } from "@/components/ui/pill";
import { RiskBadge } from "@/components/ui/risk-badge";
import { Wordmark } from "@/components/ui/wordmark";

import { FloatCard } from "./mockups";

const AUDIENCES = [
  {
    key: "analysts",
    label: "Fraud analysts",
    icon: Investigate,
    tag: "Investigate faster",
    title: "A queue that tells you",
    accent: "why.",
    body: "Every alert arrives with its score, band and the signals behind it. Triage from the keyboard, open a case in one step and keep the evidence together.",
    cta: { label: "See alerts and cases", href: "#features" },
  },
  {
    key: "risk",
    label: "Risk managers",
    icon: Alert,
    tag: "Tune with confidence",
    title: "Change strategy without",
    accent: "surprises.",
    body: "Write rules as typed conditions, run them in shadow mode first and ship them through a second pair of eyes. Every version can be rolled back.",
    cta: { label: "See rules and approvals", href: "#features" },
  },
  {
    key: "developers",
    label: "Developers",
    icon: Code,
    tag: "Integrate in an afternoon",
    title: "One API call per",
    accent: "decision.",
    body: "Send events, get an explainable decision back synchronously. Separate test and production keys, signed webhooks and an event explorer for debugging.",
    cta: { label: "See the developer hub", href: "#features" },
  },
  {
    key: "compliance",
    label: "Compliance",
    icon: Report,
    tag: "Defend every call",
    title: "An audit trail that",
    accent: "holds up.",
    body: "Material actions are audited, decisions keep their reasons, and access to tenant data needs a second staff member and ends on its own.",
    cta: { label: "Read about security", href: "#overview" },
  },
] as const;

/** "Who it's for": pill tabs with a panel per audience and a collage of floating UI. */
export function Audience() {
  return (
    <Tabs defaultValue="analysts">
      <TabList
        aria-label="Who it's for"
        className="mx-auto mb-10 justify-center"
      >
        {AUDIENCES.map((a) => (
          <Tab key={a.key} value={a.key}>
            {a.label}
          </Tab>
        ))}
      </TabList>
      {AUDIENCES.map((a) => (
        <TabPanel key={a.key} value={a.key}>
          <div className="grid items-center gap-10 rounded-xl border border-border-subtle bg-bg-surface p-6 shadow-card md:p-10 lg:grid-cols-2">
            <div>
              <OutlinePill>
                <a.icon className="size-3.5" aria-hidden="true" />
                {a.tag}
              </OutlinePill>
              <h3 className="mt-5 text-display">
                {a.title} <span className="accent-italic">{a.accent}</span>
              </h3>
              <p className="mt-4 max-w-md text-body-lg text-text-secondary">
                {a.body}
              </p>
              <Button className="mt-6" arrow asChild>
                <Link href={a.cta.href as "/"}>{a.cta.label}</Link>
              </Button>
            </div>
            <Collage />
          </div>
        </TabPanel>
      ))}
    </Tabs>
  );
}

/** A soft gradient "photo" panel with floating cards around it. */
function Collage() {
  return (
    <div aria-hidden="true" className="relative mx-auto h-80 w-full max-w-md">
      <div className="bg-feature absolute inset-x-8 inset-y-6 flex items-center justify-center overflow-hidden rounded-xl border border-border-subtle">
        <div className="bg-grid absolute inset-0" />
        <span className="relative rounded-xl bg-bg-surface px-6 py-4 shadow-glow">
          <Wordmark className="text-display" />
        </span>
      </div>
      <FloatCard className="absolute top-0 left-0 w-48">
        <p className="text-label text-text-muted">Open alert</p>
        <p className="mt-1 text-label font-semibold">Card testing burst</p>
        <div className="mt-2">
          <RiskBadge score={96} />
        </div>
      </FloatCard>
      <FloatCard className="absolute right-0 bottom-10 w-44">
        <p className="text-label text-text-muted">Decision</p>
        <div className="mt-1.5 flex items-center justify-between">
          <span className="text-heading-3 font-bold">Step-up</span>
          <StatusPill tone="info">Sent</StatusPill>
        </div>
      </FloatCard>
      <FloatCard className="absolute bottom-0 left-6 w-40">
        <p className="text-label text-text-muted">Case</p>
        <div className="mt-1.5">
          <StatusPill tone="success">Resolved</StatusPill>
        </div>
      </FloatCard>
    </div>
  );
}
