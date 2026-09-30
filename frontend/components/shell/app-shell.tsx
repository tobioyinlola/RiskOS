import type { ReactNode } from "react";

import { MobileNav } from "./mobile-nav";
import { NavLinks } from "./nav-links";

/**
 * Console layout: a persistent sidebar on large screens and a slide-over menu on small ones.
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh">
      <aside className="hidden w-60 shrink-0 border-r bg-sidebar text-sidebar-foreground lg:flex lg:flex-col">
        <div className="flex h-14 items-center border-b px-5 font-semibold tracking-tight">
          RiskOS
        </div>
        <nav aria-label="Primary" className="flex-1 overflow-y-auto p-3">
          <NavLinks />
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center gap-2 border-b px-4 lg:hidden">
          <MobileNav />
          <span className="font-semibold tracking-tight">RiskOS</span>
        </header>
        <main className="flex-1 p-4 sm:p-6 lg:p-8">{children}</main>
      </div>
    </div>
  );
}
