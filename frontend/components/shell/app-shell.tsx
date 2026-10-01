"use client";

import { useQueryClient } from "@tanstack/react-query";
import { cn } from "cn";
import {
  Activity,
  ChevronsLeft,
  ChevronsRight,
  Code,
  FolderOpen,
  Gauge,
  Keyboard,
  LogOut,
  Menu,
  Moon,
  Rows3,
  Rows4,
  ScrollText,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sun,
  TriangleAlert,
  UserRound,
  Users,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { DropdownMenu as M } from "radix-ui";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { Modal } from "@/components/ui/overlay";
import { Tooltip, TooltipProvider } from "@/components/ui/tooltip";
import { Wordmark } from "@/components/ui/wordmark";
import { api } from "@/lib/api/client";
import {
  isActiveSection,
  visibleSections,
  type NavSection,
} from "@/lib/navigation";
import {
  getPreferences,
  setPreferences,
  usePreferences,
} from "@/lib/preferences";
import { useSession } from "@/lib/session";

import { CommandPalette, type Command } from "./command-palette";
import { isTyping, useShortcuts } from "./shortcuts";

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

const GROUPS = ["Monitor", "Configure", "Organization"] as const;

function NavLinks({
  collapsed = false,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const { can } = useSession();
  const sections = visibleSections(can);
  return (
    <nav aria-label="Primary" className="flex flex-col gap-5">
      {GROUPS.map((g) => {
        const items = sections.filter((s) => s.group === g);
        if (items.length === 0) return null;
        return (
          <div key={g}>
            {collapsed ? (
              <div className="mx-3 mb-2 border-t border-border-subtle" />
            ) : (
              <p className="mb-1 px-3 text-overline text-text-secondary uppercase">
                {g}
              </p>
            )}
            <ul className="flex flex-col gap-0.5">
              {items.map((s) => {
                const Icon = ICONS[s.icon];
                const active = isActiveSection(s.href, pathname);
                return (
                  <li key={s.href}>
                    <Tooltip content={s.label} disabled={!collapsed}>
                      <Link
                        href={s.href}
                        onClick={onNavigate}
                        aria-current={active ? "page" : undefined}
                        aria-label={collapsed ? s.label : undefined}
                        className={cn(
                          "flex h-control-md items-center gap-3 rounded-md text-body transition-colors duration-150 ease-out",
                          collapsed ? "justify-center px-0" : "px-3",
                          active
                            ? "bg-accent-subtle font-medium text-text-primary"
                            : "text-text-secondary hover:bg-bg-raised hover:text-text-primary",
                        )}
                      >
                        <Icon
                          className={cn(
                            "size-icon-md shrink-0 stroke-[1.5]",
                            active ? "text-accent" : "",
                          )}
                          aria-hidden="true"
                        />
                        {collapsed ? null : (
                          <span className="truncate">{s.label}</span>
                        )}
                      </Link>
                    </Tooltip>
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
      className="inline-flex rounded-md border border-border-control p-0.5"
    >
      {(["test", "production"] as const).map((env) => (
        <button
          key={env}
          type="button"
          role="radio"
          aria-checked={environment === env}
          onClick={() => setEnvironment(env)}
          className={cn(
            "h-5 rounded-sm px-2.5 text-label transition-colors duration-150 ease-out",
            environment === env
              ? env === "production"
                ? "bg-risk-elevated-bg text-status-warning"
                : "bg-accent-subtle text-text-primary"
              : "text-text-muted hover:text-text-primary",
          )}
        >
          {env === "test" ? "Test" : "Production"}
        </button>
      ))}
    </div>
  );
}

function useSignOut() {
  const router = useRouter();
  const qc = useQueryClient();
  return useCallback(async () => {
    await api().POST("/api/v1/auth/logout");
    qc.clear();
    router.replace("/login");
  }, [qc, router]);
}

const menuItem =
  "flex w-full cursor-pointer items-center gap-2 rounded-sm px-3 py-2 text-left text-body outline-none data-[highlighted]:bg-bg-surface [&_svg]:size-icon-sm [&_svg]:stroke-[1.5]";

function UserMenu() {
  const { session } = useSession();
  const router = useRouter();
  const signOut = useSignOut();
  const prefs = usePreferences();
  return (
    <M.Root>
      <M.Trigger asChild>
        <Button variant="ghost" size="sm" aria-label={session.user.name}>
          <UserRound />
          <span className="hidden max-w-40 truncate sm:inline">
            {session.user.name}
          </span>
        </Button>
      </M.Trigger>
      <M.Portal>
        <M.Content
          align="end"
          sideOffset={6}
          className="z-50 w-64 rounded-md border border-border-subtle bg-bg-raised p-1 shadow-overlay"
          style={{ animation: "fade-in 120ms ease-out" }}
        >
          <div className="px-3 py-2">
            <p className="text-body">{session.user.name}</p>
            <p className="mono truncate text-data text-text-muted">
              {session.user.email}
            </p>
            <p className="mt-1 text-label text-text-secondary">
              {session.user.role.replace("_", " ")} · {session.tenant.name}
            </p>
          </div>
          <M.Separator className="my-1 border-t border-border-subtle" />
          <M.Item className={menuItem} onSelect={() => router.push("/account")}>
            <ShieldCheck /> Account security
          </M.Item>
          <M.Item
            className={menuItem}
            onSelect={() =>
              setPreferences({
                theme: prefs.theme === "dark" ? "light" : "dark",
              })
            }
          >
            {prefs.theme === "dark" ? <Sun /> : <Moon />}
            {prefs.theme === "dark" ? "Use light theme" : "Use dark theme"}
          </M.Item>
          <M.Separator className="my-1 border-t border-border-subtle" />
          <M.Item className={menuItem} onSelect={() => void signOut()}>
            <LogOut /> Sign out
          </M.Item>
        </M.Content>
      </M.Portal>
    </M.Root>
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
          { keys: "Ctrl K", label: "Command palette" },
          { keys: "[", label: "Collapse or expand navigation" },
          { keys: "?", label: "Show this help" },
          { keys: "g o", label: "Go to overview" },
          { keys: "g a", label: "Go to alerts" },
          { keys: "g e", label: "Go to events" },
          { keys: "g c", label: "Go to cases" },
          { keys: "g r", label: "Go to rules" },
        ].map((b) => (
          <li
            key={b.keys + b.label}
            className="flex items-center justify-between border-t border-border-subtle py-2 text-body-sm"
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

function useCommands(openHelp: () => void): Command[] {
  const router = useRouter();
  const { can, environment, setEnvironment } = useSession();
  const prefs = usePreferences();
  const signOut = useSignOut();
  return useMemo(() => {
    const go: Command[] = visibleSections(can).map((s) => {
      const Icon = ICONS[s.icon];
      return {
        id: `go-${s.href}`,
        label: s.label,
        group: "Go to",
        icon: <Icon />,
        keywords: s.group,
        run: () => router.push(s.href),
      };
    });
    const other = environment === "test" ? "production" : "test";
    return [
      ...go,
      {
        id: "go-account",
        label: "Account security",
        group: "Go to",
        icon: <ShieldCheck />,
        keywords: "passkey mfa password recovery",
        run: () => router.push("/account"),
      },
      {
        id: "env",
        label: `Switch to ${other} environment`,
        group: "Actions",
        icon: <TriangleAlert />,
        keywords: "environment test production live",
        run: () => setEnvironment(other),
      },
      {
        id: "theme",
        label: prefs.theme === "dark" ? "Use light theme" : "Use dark theme",
        group: "Actions",
        icon: prefs.theme === "dark" ? <Sun /> : <Moon />,
        keywords: "theme dark light daylight console appearance",
        run: () =>
          setPreferences({ theme: prefs.theme === "dark" ? "light" : "dark" }),
      },
      {
        id: "density",
        label:
          prefs.density === "default" ? "Use compact rows" : "Use default rows",
        group: "Actions",
        icon: prefs.density === "default" ? <Rows4 /> : <Rows3 />,
        keywords: "density compact rows table",
        run: () =>
          setPreferences({
            density: prefs.density === "default" ? "compact" : "default",
          }),
      },
      {
        id: "sidebar",
        label: prefs.sidebarCollapsed
          ? "Expand navigation"
          : "Collapse navigation",
        group: "Actions",
        icon: prefs.sidebarCollapsed ? <ChevronsRight /> : <ChevronsLeft />,
        shortcut: "[",
        keywords: "sidebar navigation menu",
        run: () =>
          setPreferences({ sidebarCollapsed: !prefs.sidebarCollapsed }),
      },
      {
        id: "help",
        label: "Keyboard shortcuts",
        group: "Actions",
        icon: <Keyboard />,
        shortcut: "?",
        run: openHelp,
      },
      {
        id: "signout",
        label: "Sign out",
        group: "Actions",
        icon: <LogOut />,
        run: () => void signOut(),
      },
    ];
  }, [can, environment, openHelp, prefs, router, setEnvironment, signOut]);
}

/**
 * Console layout (docs/design-system.md, Spacing and layout): a 240px left navigation that
 * collapses to a 64px icon rail, a top bar with the environment switch, search and the user
 * menu, and the page. Production gets a standing banner.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { environment, session } = useSession();
  const { setHelpOpen } = useShortcuts();
  const prefs = usePreferences();
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const openHelp = useCallback(() => setHelpOpen(true), [setHelpOpen]);
  const commands = useCommands(openHelp);
  const collapsed = prefs.sidebarCollapsed;

  // Close the mobile navigation whenever the page changes.
  const [lastPath, setLastPath] = useState(pathname);
  if (lastPath !== pathname) {
    setLastPath(pathname);
    setMobileOpen(false);
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
        return;
      }
      if (isTyping(e) || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "[") {
        e.preventDefault();
        setPreferences({
          sidebarCollapsed: !getPreferences().sidebarCollapsed,
        });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <TooltipProvider>
      <div className="flex min-h-dvh bg-bg-canvas">
        <aside
          className={cn(
            "sticky top-0 hidden h-dvh shrink-0 flex-col border-r border-border-subtle bg-bg-surface transition-[width] duration-200 ease-out lg:flex",
            collapsed ? "w-rail" : "w-nav",
          )}
          aria-label="Navigation"
        >
          <div
            className={cn(
              "flex h-14 items-center border-b border-border-subtle",
              collapsed ? "justify-center px-2" : "justify-between px-4",
            )}
          >
            <Link href="/" aria-label="RiskOS overview">
              <Wordmark compact={collapsed} />
            </Link>
          </div>
          <div
            className={cn(
              "flex-1 overflow-x-hidden overflow-y-auto py-4",
              collapsed ? "px-2" : "px-3",
            )}
          >
            <NavLinks collapsed={collapsed} />
          </div>
          <div
            className={cn(
              "flex items-center border-t border-border-subtle py-2",
              collapsed ? "justify-center px-2" : "justify-between px-4",
            )}
          >
            {collapsed ? null : (
              <span className="truncate text-label text-text-muted">
                {session.tenant.name}
              </span>
            )}
            <Tooltip
              content={
                collapsed ? "Expand navigation  [" : "Collapse navigation  ["
              }
            >
              <Button
                variant="ghost"
                size="icon"
                aria-label={
                  collapsed ? "Expand navigation" : "Collapse navigation"
                }
                aria-expanded={!collapsed}
                aria-keyshortcuts="["
                onClick={() => setPreferences({ sidebarCollapsed: !collapsed })}
              >
                {collapsed ? <ChevronsRight /> : <ChevronsLeft />}
              </Button>
            </Tooltip>
          </div>
        </aside>

        {mobileOpen ? (
          <div
            className="fixed inset-0 z-40 flex bg-bg-canvas/70 lg:hidden"
            style={{ animation: "fade-in 150ms ease-out" }}
          >
            <div className="flex w-nav flex-col border-r border-border-subtle bg-bg-surface shadow-overlay">
              <div className="flex h-14 items-center justify-between border-b border-border-subtle px-4">
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
              <div className="flex-1 overflow-y-auto px-3 py-4">
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
              className="flex h-8 items-center gap-2 border-b border-status-warning bg-risk-elevated-bg px-4 lg:px-6"
            >
              <TriangleAlert
                className="size-icon-sm stroke-[1.5] text-status-warning"
                aria-hidden="true"
              />
              <span className="text-label text-status-warning">
                Production: live data and live keys
              </span>
            </div>
          ) : null}
          <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-border-subtle bg-bg-surface px-4 lg:px-6">
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
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="ml-2 hidden h-control-sm w-64 items-center gap-2 rounded-md border border-border-subtle bg-bg-sunken px-3 text-body-sm text-text-muted transition-colors duration-150 hover:border-border-control hover:text-text-secondary md:flex"
              aria-label="Open command palette"
              aria-keyshortcuts="Control+K Meta+K"
            >
              <Search
                className="size-icon-sm stroke-[1.5]"
                aria-hidden="true"
              />
              <span className="flex-1 text-left">Search or jump to</span>
              <Kbd>Ctrl K</Kbd>
            </button>
            <div className="ml-auto flex items-center gap-1">
              <Tooltip
                side="bottom"
                content={
                  prefs.density === "default" ? "Compact rows" : "Default rows"
                }
              >
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={
                    prefs.density === "default"
                      ? "Use compact rows"
                      : "Use default rows"
                  }
                  aria-pressed={prefs.density === "compact"}
                  onClick={() =>
                    setPreferences({
                      density:
                        prefs.density === "default" ? "compact" : "default",
                    })
                  }
                >
                  {prefs.density === "default" ? <Rows4 /> : <Rows3 />}
                </Button>
              </Tooltip>
              <Tooltip
                side="bottom"
                content={prefs.theme === "dark" ? "Light theme" : "Dark theme"}
              >
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={
                    prefs.theme === "dark"
                      ? "Use light theme"
                      : "Use dark theme"
                  }
                  onClick={() =>
                    setPreferences({
                      theme: prefs.theme === "dark" ? "light" : "dark",
                    })
                  }
                >
                  {prefs.theme === "dark" ? <Sun /> : <Moon />}
                </Button>
              </Tooltip>
              <Tooltip side="bottom" content="Keyboard shortcuts  ?">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Keyboard shortcuts"
                  onClick={openHelp}
                >
                  <Keyboard />
                </Button>
              </Tooltip>
              <UserMenu />
            </div>
          </header>
          <main
            key={pathname}
            className="flex-1 px-4 py-5 lg:px-6"
            style={{ animation: "fade-in 150ms ease-out" }}
          >
            {children}
          </main>
        </div>
        <ShortcutsHelp />
        <CommandPalette
          open={paletteOpen}
          onOpenChange={setPaletteOpen}
          commands={commands}
        />
      </div>
    </TooltipProvider>
  );
}
