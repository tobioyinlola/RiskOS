"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/wordmark";
import { DOCS_NAV } from "@/lib/docs";

function NavLinks({ onPick }: { onPick?: () => void }) {
  const path = usePathname();
  const active = (href: string) =>
    href === "/docs"
      ? path === "/docs"
      : path === href || path.startsWith(`${href}/`);
  return (
    <div className="flex flex-col gap-6">
      {DOCS_NAV.map((section) => (
        <div key={section.title}>
          <p className="mb-2 text-label font-semibold tracking-wide text-text-muted uppercase">
            {section.title}
          </p>
          <ul className="flex flex-col gap-0.5">
            {section.links.map((l) => (
              <li key={l.href}>
                <Link
                  href={l.href}
                  onClick={onPick}
                  aria-current={active(l.href) ? "page" : undefined}
                  className={`block rounded-md px-2.5 py-1.5 text-body-sm transition-colors ${
                    active(l.href)
                      ? "bg-bg-raised font-medium text-text-primary"
                      : "text-text-secondary hover:bg-bg-sunken hover:text-text-primary"
                  }`}
                >
                  {l.label}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

/** The developer documentation: site header, section navigation and the page. */
export function DocsShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-dvh bg-bg-surface text-text-primary">
      <header className="sticky top-0 z-20 border-b border-border-subtle bg-bg-surface/90 backdrop-blur-md">
        <nav
          aria-label="Site"
          className="mx-auto flex max-w-7xl items-center gap-4 px-4 py-3"
        >
          <Link href="/home" aria-label="RiskOS home">
            <Wordmark className="text-heading-3" />
          </Link>
          <Link
            href="/docs"
            className="hidden text-body-sm font-medium text-text-secondary hover:text-text-primary sm:inline"
          >
            Developer docs
          </Link>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="ghost" size="sm" asChild>
              <Link href="/login">Log in</Link>
            </Button>
            <Button variant="primary" size="sm" asChild>
              <Link href="/signup">Create a workspace</Link>
            </Button>
          </div>
        </nav>
      </header>
      <div className="mx-auto grid max-w-7xl grid-cols-1 gap-8 px-4 lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="hidden py-8 lg:block">
          <nav aria-label="Documentation" className="sticky top-20">
            <NavLinks />
          </nav>
        </aside>
        <details className="mt-4 rounded-md border border-border-subtle lg:hidden">
          <summary className="cursor-pointer px-4 py-3 text-body-sm font-medium">
            Documentation menu
          </summary>
          <nav aria-label="Documentation" className="px-4 pb-4">
            <NavLinks />
          </nav>
        </details>
        <main id="content" className="min-w-0 pt-6 pb-24 lg:py-10">
          {children}
        </main>
      </div>
    </div>
  );
}
