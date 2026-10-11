import Link from "next/link";
import type { ReactNode } from "react";

import { AvatarStack } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Alert,
  Code,
  GitHub,
  Investigate,
  Report,
  Sparkle,
} from "@/components/ui/icons";
import { OutlinePill } from "@/components/ui/pill";
import { Wordmark } from "@/components/ui/wordmark";

import { Audience } from "./audience";
import { DashboardPreview, PhonePreview, SNIPPETS } from "./mockups";
import { Newsletter } from "./newsletter";

/** The repository: the one public link the project has today. */
const GITHUB_URL = "https://github.com/tobioyinlola/RiskOS";

/** The roles RiskOS serves, shown as an avatar stack (people we describe, not customers). */
const ROLES = [
  { name: "Fraud analysts", icon: <Investigate /> },
  { name: "Risk managers", icon: <Alert /> },
  { name: "Developers", icon: <Code /> },
  { name: "Compliance", icon: <Report /> },
];

const NAV_LINKS = [
  { href: "#overview", label: "Overview" },
  { href: "#features", label: "Features" },
  { href: "#audience", label: "Who it's for" },
];

/** "Bold phrase *lighter italic phrase*", the signature heading. */
function Headline({
  children,
  accent,
  as: Tag = "h2",
  className = "text-display",
}: {
  children: ReactNode;
  accent: string;
  as?: "h1" | "h2";
  className?: string;
}) {
  return (
    <Tag className={className}>
      {children} <span className="accent-italic">{accent}</span>
    </Tag>
  );
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <span className="shrink-0 text-body-sm text-text-muted">– {children}</span>
  );
}

/** The marketing site at /home (docs/design-system.md, Landing). */
export function Landing() {
  return (
    <div className="min-h-dvh bg-bg-surface text-text-primary">
      {/* Hero, with the floating pill navbar over its wash. */}
      <section className="bg-hero relative overflow-hidden">
        <div className="bg-grid fade-radial pointer-events-none absolute inset-0" />
        <header className="relative z-10 px-4 pt-5">
          <nav
            aria-label="Site"
            className="mx-auto flex max-w-5xl items-center gap-4 rounded-full border border-border-subtle bg-bg-surface/85 py-2 pr-2 pl-4 shadow-card backdrop-blur-md"
          >
            <Link href="/home" aria-label="RiskOS home">
              <Wordmark className="text-heading-3" />
            </Link>
            <ul className="mx-auto hidden items-center gap-6 md:flex">
              {NAV_LINKS.map((l) => (
                <li key={l.href}>
                  <a
                    href={l.href}
                    className="text-body text-text-secondary transition-colors hover:text-text-primary"
                  >
                    {l.label}
                  </a>
                </li>
              ))}
            </ul>
            <div className="ml-auto flex items-center gap-2 md:ml-0">
              <Button variant="ghost" size="sm" asChild>
                <Link href="/login">Log in</Link>
              </Button>
              <Button variant="primary" size="sm" arrow asChild>
                <Link href="/signup">Get started</Link>
              </Button>
            </div>
          </nav>
        </header>

        <div className="relative mx-auto max-w-5xl px-4 pt-20 text-center md:pt-28">
          <OutlinePill className="mb-6">
            <Sparkle className="size-3.5 text-accent" aria-hidden="true" />
            Explainable risk decisions for financial events
          </OutlinePill>
          <h1 className="mx-auto max-w-4xl text-[44px] leading-[1.05] font-bold tracking-[-0.035em] md:text-hero">
            Stop fraud before it{" "}
            <span className="rounded-2xl relative inline-block bg-bg-surface px-4 pb-1 align-baseline shadow-glow">
              <span className="accent-italic">settles.</span>
            </span>
          </h1>
          <p className="mx-auto mt-6 max-w-2xl text-body-lg text-text-secondary">
            RiskOS scores every payment, login and payout as it happens,
            explains the score, and routes the risky ones to your analysts:
            allow, step-up, review or block.
          </p>
          <div className="mt-8 flex flex-wrap justify-center gap-3">
            {/* "Request a demo" in the design: it needs a destination (a form or a calendar). */}
            <Button variant="primary" size="lg" arrow asChild>
              <Link href="/signup">Get started</Link>
            </Button>
            <Button size="lg" asChild>
              <a href="#features">Explore features</a>
            </Button>
          </div>
        </div>
        <div className="fade-bottom relative mx-auto mt-16 max-w-6xl px-4">
          <DashboardPreview />
        </div>
      </section>

      {/* Overview. */}
      <section
        id="overview"
        className="mx-auto max-w-6xl scroll-mt-8 px-4 py-24"
      >
        <div className="flex flex-wrap items-end justify-between gap-4 border-b border-border-subtle pb-6">
          <Headline accent="explained.">Every decision,</Headline>
          <SectionLabel>Overview</SectionLabel>
        </div>
        <div className="mt-10 grid gap-10 lg:grid-cols-[260px_minmax(0,1fr)]">
          <div>
            <AvatarStack people={ROLES} size="lg" />
            <p className="mt-3 text-body-sm text-text-secondary">
              Built for the people who own risk
            </p>
          </div>
          <div>
            <p className="text-[28px] leading-[1.3] font-medium tracking-[-0.02em]">
              RiskOS turns raw financial events into a score, a band and a
              decision{" "}
              <span className="text-text-faint">
                with the signals behind it,
              </span>{" "}
              so your team acts in seconds{" "}
              <span className="text-text-faint">
                and can defend every call it makes.
              </span>
            </p>
            <Button variant="primary" className="mt-8" arrow asChild>
              <a href="#features">See how it works</a>
            </Button>
          </div>
        </div>
        <div className="relative mt-16 flex justify-center">
          <svg
            aria-hidden="true"
            viewBox="0 0 1200 300"
            preserveAspectRatio="none"
            className="absolute inset-x-0 top-1/2 h-64 w-full -translate-y-1/2"
          >
            <path
              d="M0 220 C 200 60, 380 280, 600 150 S 1000 20, 1200 120"
              fill="none"
              stroke="var(--brand-blue)"
              strokeWidth="3"
              strokeLinecap="round"
            />
            <path
              d="M0 250 C 220 100, 400 300, 600 190 S 1000 70, 1200 160"
              fill="none"
              stroke="var(--blue-200)"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
          <div className="relative">
            <PhonePreview />
          </div>
        </div>
      </section>

      {/* Features bento. */}
      <section id="features" className="scroll-mt-8 bg-bg-canvas py-24">
        <div className="mx-auto max-w-6xl px-4">
          <div className="mb-12 flex flex-wrap items-end justify-between gap-4">
            <Headline accent="risk teams.">
              Powerful features built for
            </Headline>
            <SectionLabel>Features</SectionLabel>
          </div>
          <div className="grid gap-5 md:grid-cols-2">
            <FeatureCard
              title="Real-time scoring"
              text="A 0–100 score, a band and a decision for every event, returned synchronously."
              snippet={SNIPPETS.scoring}
            />
            <FeatureCard
              title="Explainable signals"
              text="Each score lists the signals that moved it and by how much."
              snippet={SNIPPETS.signals}
            />
          </div>
          <div className="mt-5 grid gap-5 md:grid-cols-3">
            <FeatureCard
              title="Alerts and cases"
              text="A keyboard-driven queue, with cases that keep the evidence together."
              snippet={SNIPPETS.alerts}
            />
            <FeatureCard
              title="Rules with approvals"
              text="Typed rules, shadow mode first, a second approver and rollback."
              snippet={SNIPPETS.rules}
            />
            <FeatureCard
              title="Developer API"
              text="Test and production keys, signed webhooks and an event explorer."
              snippet={SNIPPETS.api}
            />
          </div>
        </div>
      </section>

      {/* Audience tabs. */}
      <section
        id="audience"
        className="mx-auto max-w-6xl scroll-mt-8 px-4 py-24"
      >
        <div className="mb-8 flex flex-col items-center text-center">
          <OutlinePill>
            <span
              aria-hidden="true"
              className="size-3 rounded-full bg-gradient-to-br from-blue-300 to-brand-blue shadow-glow"
            />
            Who it&apos;s for
          </OutlinePill>
          <Headline accent="every team." className="mt-5 text-display">
            One platform for
          </Headline>
        </div>
        <Audience />
      </section>

      <Footer />
    </div>
  );
}

function FeatureCard({
  title,
  text,
  snippet,
}: {
  title: string;
  text: string;
  snippet: ReactNode;
}) {
  return (
    <article className="bg-feature flex min-h-80 flex-col overflow-hidden rounded-xl border border-border-subtle">
      <div className="p-7">
        <h3 className="text-heading-2 font-bold">{title}</h3>
        <p className="mt-2 max-w-sm text-body text-text-secondary">{text}</p>
      </div>
      <div className="mt-auto flex justify-center px-7 pb-0">
        <div className="w-full translate-y-3">{snippet}</div>
      </div>
    </article>
  );
}

function Footer() {
  const columns = [
    {
      title: "Sitemap",
      links: [
        { href: "#overview", label: "Overview" },
        { href: "#features", label: "Features" },
        { href: "#audience", label: "Who it's for" },
        { href: "/login", label: "Log in" },
      ],
    },
    {
      title: "Product",
      links: [
        { href: "#features", label: "Real-time scoring" },
        { href: "#features", label: "Alerts and cases" },
        { href: "#features", label: "Rules" },
        { href: "/docs", label: "Developer API" },
      ],
    },
    {
      title: "Resources",
      links: [
        { href: "/docs", label: "Developer docs" },
        { href: "/docs/reference", label: "API reference" },
        { href: GITHUB_URL, label: "Source code" },
        { href: `${GITHUB_URL}/blob/main/docs/SECURITY.md`, label: "Security" },
      ],
    },
  ];
  return (
    <footer className="relative overflow-hidden border-t border-border-subtle bg-bg-surface">
      <div className="mx-auto max-w-6xl px-4 pt-24">
        <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div>
            <Headline
              accent="count."
              className="text-display md:text-[56px] md:leading-[1.05]"
            >
              Make every risk decision
            </Headline>
            <AvatarStack
              className="mt-6"
              people={ROLES}
              caption="Built for fraud, risk and compliance teams"
            />
          </div>
          <Button variant="primary" size="lg" arrow asChild>
            <Link href="/signup">Create your workspace</Link>
          </Button>
        </div>

        <div className="mt-16 grid gap-10 border-t border-border-subtle pt-12 md:grid-cols-[repeat(3,minmax(0,1fr))_minmax(0,1.4fr)]">
          {columns.map((c) => (
            <div key={c.title}>
              <h3 className="mb-4 text-body-sm font-semibold">{c.title}</h3>
              <ul className="flex flex-col gap-2.5">
                {c.links.map((l) => (
                  <li key={l.label}>
                    <a
                      href={l.href}
                      className="text-body-sm text-text-secondary transition-colors hover:text-text-primary"
                    >
                      {l.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
          <div>
            <h3 className="mb-4 text-body-sm font-semibold">
              Stay in the loop
            </h3>
            <Newsletter />
            <div className="mt-6 flex gap-2">
              <a
                href={GITHUB_URL}
                aria-label="RiskOS on GitHub"
                className="flex size-10 items-center justify-center rounded-md border border-border-strong text-text-secondary transition-colors hover:bg-bg-raised hover:text-text-primary"
              >
                <GitHub className="size-icon-md" />
              </a>
            </div>
          </div>
        </div>

        <div className="mt-12 flex flex-wrap items-center justify-between gap-3 border-t border-border-subtle py-6 text-label text-text-muted">
          <span className="flex items-center gap-2">
            © {new Date().getFullYear()} RiskOS
          </span>
          <span>Explainable risk decisions for financial events</span>
        </div>
      </div>
      <p
        aria-hidden="true"
        data-decorative
        className="pointer-events-none -mb-[0.22em] text-center text-[clamp(96px,22vw,320px)] leading-none font-bold tracking-[-0.06em] text-bg-sunken select-none"
      >
        RiskOS
      </p>
    </footer>
  );
}
