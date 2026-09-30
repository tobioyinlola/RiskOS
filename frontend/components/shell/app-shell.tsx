"use client";

import { cn } from "cn";
import {
  Activity,
  Code,
  FolderOpen,
  Gauge,
  LogOut,
  Menu,
  ScrollText,
  ShieldCheck,
  SlidersHorizontal,
  TriangleAlert,
  UserRound,
  Users,
  X,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";

import { Button } from "@/components/instrument/button";
import { Kbd } from "@/components/instrument/kbd";
import { Modal } from "@/components/instrument/overlay";
import { Wordmark } from "@/components/instrument/symbol";
import { api } from "@/lib/api/client";
import {
  isActiveSection,
  visibleSections,
  type NavSection,
} from "@/lib/navigation";
import { useSession } from "@/lib/session";

import { useShortcuts } from "./shortcuts";

const ICONS: Record<NavSection["icon"], typeof Gauge> = {
  gauge: Gauge,
  activity: Activity,
  alert: TriangleAlert,
  folder: FolderOpen,
  shield: ShieldCheck,
  sliders: SlidersHorizontal,
  code: Code,
  users: Users,
  scroll: ScrollText,
};

function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  const { can } = useSession();
  const sections = visibleSections(can);
  const groups = ["Monitor", "Configure", "Organization"] as const;
  return (
    <nav aria-label="Primary" className="flex flex-col gap-5">
      {groups.map((g) => {
        const items = sections.filter((s) => s.group === g);
        if (items.length === 0) return null;
        return (
          <div key={g}>
            <p className="legend mb-1.5 px-3">{g}</p>
            <ul className="flex flex-col gap-px">
              {items.map((s) => {
                const Icon = ICONS[s.icon];
                const active = isActiveSection(s.href, pathname);
                return (
                  <li key={s.href}>
                    <Link
                      href={s.href}
                      onClick={onNavigate}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex h-8 items-center gap-2.5 rounded-md px-3 text-body",
                        active
                          ? "bg-signal-wash text-ink shadow-[inset_2px_0_0_var(--signal-ink)]"
                          : "text-ink-2 hover:bg-bg-raised hover:text-ink",
                      )}
                    >
                      <Icon
                        className="size-5 shrink-0 stroke-[1.5]"
                        strokeLinecap="square"
                      />
                      {s.label}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        );
      })}
    </nav>
  );
}

function EnvironmentSwitch() {
  const { environment, setEnvironment } = useSession();
  return (
    <div
      role="radiogroup"
      aria-label="Environment"
      className="inline-flex rounded-md border border-line-control p-0.5"
    >
      {(["test", "production"] as const).map((env) => (
        <button
          key={env}
          type="button"
          role="radio"
          aria-checked={environment === env}
          onClick={() => setEnvironment(env)}
          className={cn(
            "h-6 rounded-sm px-2.5 text-label",
            environment === env
              ? "bg-bg-raised text-ink"
              : "text-ink-3 hover:text-ink",
          )}
        >
          {env === "test" ? "Test" : "Production"}
        </button>
      ))}
    </div>
  );
}

function UserMenu() {
  const { session } = useSession();
  const router = useRouter();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const signOut = async () => {
    await api().POST("/api/v1/auth/logout");
    qc.clear();
    router.replace("/login");
  };
  return (
    <div className="relative">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="menu"
      >
        <UserRound className="size-4 stroke-[1.5]" />
        <span className="max-w-40 truncate">{session.user.name}</span>
      </Button>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 z-30 mt-1 w-64 rounded-md border border-line-subtle bg-bg-raised p-1 shadow-overlay"
          onMouseLeave={() => setOpen(false)}
        >
          <div className="px-3 py-2">
            <p className="text-body">{session.user.name}</p>
            <p className="mono truncate text-data text-ink-3">
              {session.user.email}
            </p>
            <p className="legend mt-1">
              {session.user.role.replace("_", " ")} · {session.tenant.name}
            </p>
          </div>
          <div className="my-1 border-t border-line-subtle" />
          <button
            role="menuitem"
            className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body hover:bg-bg-surface"
            onClick={() => {
              setOpen(false);
              router.push("/account");
            }}
          >
            <ShieldCheck className="size-4 stroke-[1.5]" /> Account security
          </button>
          <button
            role="menuitem"
            className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body hover:bg-bg-surface"
            onClick={() => void signOut()}
          >
            <LogOut className="size-4 stroke-[1.5]" /> Sign out
          </button>
        </div>
      ) : null}
    </div>
  );
}

function ShortcutsHelp() {
  const { helpOpen, setHelpOpen, bindings } = useShortcuts();
  return (
    <Modal
      open={helpOpen}
      onOpenChange={setHelpOpen}
      title="Keyboard shortcuts"
    >
      <ul className="flex flex-col">
        {[
          ...bindings,
          { keys: "?", label: "Show this help" },
          { keys: "g a", label: "Go to alerts" },
          { keys: "g e", label: "Go to events" },
          { keys: "g c", label: "Go to cases" },
        ].map((b) => (
          <li
            key={b.keys + b.label}
            className="flex items-center justify-between border-t border-line-subtle py-2 text-body-sm"
          >
            <span>{b.label}</span>
            <span className="flex gap-1">
              {b.keys.split(" ").map((k) => (
                <Kbd key={k}>{k}</Kbd>
              ))}
            </span>
          </li>
        ))}
      </ul>
    </Modal>
  );
}

/**
 * Console layout (docs/design-system.md): a 232px left navigation, a top bar with the
 * environment switch, and the page. Production gets a standing banner.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { environment, session } = useSession();
  const [mobileOpen, setMobileOpen] = useState(false);
  return (
    <div className="flex min-h-dvh bg-bg-canvas">
      <aside className="hidden w-nav shrink-0 flex-col border-r border-line-subtle bg-bg-surface lg:flex">
        <div className="flex h-14 items-center border-b border-line-subtle px-5">
          <Link href="/" aria-label="RiskOS overview">
            <Wordmark />
          </Link>
        </div>
        <div className="flex-1 overflow-y-auto px-2 py-4">
          <NavLinks />
        </div>
        <p className="legend border-t border-line-subtle px-5 py-3">
          {session.tenant.name}
        </p>
      </aside>

      {mobileOpen ? (
        <div className="fixed inset-0 z-40 flex bg-bg-canvas/70 lg:hidden">
          <div className="flex w-nav flex-col border-r border-line-subtle bg-bg-surface shadow-overlay">
            <div className="flex h-14 items-center justify-between border-b border-line-subtle px-4">
              <Wordmark />
              <Button
                variant="ghost"
                size="icon"
                aria-label="Close navigation"
                onClick={() => setMobileOpen(false)}
              >
                <X />
              </Button>
            </div>
            <div className="flex-1 overflow-y-auto px-2 py-4">
              <NavLinks onNavigate={() => setMobileOpen(false)} />
            </div>
          </div>
          <button
            className="flex-1"
            aria-label="Close navigation"
            onClick={() => setMobileOpen(false)}
          />
        </div>
      ) : null}

      <div className="flex min-w-0 flex-1 flex-col">
        {environment === "production" ? (
          <div
            role="status"
            className="flex h-7 items-center gap-2 border-b border-status-warn bg-risk-elevated-wash px-6"
          >
            <TriangleAlert
              className="size-3.5 stroke-[1.5] text-status-warn"
              aria-hidden="true"
            />
            <span className="legend text-status-warn">
              Production · live data and live keys
            </span>
          </div>
        ) : null}
        <header className="flex h-14 items-center gap-3 border-b border-line-subtle bg-bg-surface px-4 lg:px-6">
          <Button
            variant="ghost"
            size="icon"
            className="lg:hidden"
            aria-label="Open navigation"
            onClick={() => setMobileOpen(true)}
          >
            <Menu />
          </Button>
          <EnvironmentSwitch />
          <div className="ml-auto flex items-center gap-2">
            <span className="legend hidden sm:inline">
              <Kbd>?</Kbd> shortcuts
            </span>
            <UserMenu />
          </div>
        </header>
        <main className="flex-1 px-4 py-6 lg:px-8">{children}</main>
      </div>
      <ShortcutsHelp />
    </div>
  );
}
