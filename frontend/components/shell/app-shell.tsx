"use client";

import { useQueryClient } from "@tanstack/react-query";
import { cn } from "@/lib/cn";
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

import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Activity,
  Alert,
  Ban,
  Bell,
  Chart,
  ChevronLeft,
  ChevronRight,
  ChevronUpDown,
  Close,
  Code,
  Fingerprint,
  Folder,
  Home,
  Key,
  Keyboard,
  Live,
  Lock,
  Menu,
  Receipt,
  Rows,
  Scroll,
  Scales,
  Search,
  ShieldCheck,
  SignOut,
  Sliders,
  Team,
  type Icon,
} from "@/components/ui/icons";
import { Kbd } from "@/components/ui/kbd";
import { Modal } from "@/components/ui/overlay";
import { Tooltip, TooltipProvider } from "@/components/ui/tooltip";
import { Monogram } from "@/components/ui/wordmark";
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
import { useAlerts } from "@/lib/queries";
import { useSession } from "@/lib/session";

import { CommandPalette, type Command } from "./command-palette";
import { isTyping, useShortcuts } from "./shortcuts";

const ICONS: Record<NavSection["icon"], Icon> = {
  gauge: Home,
  radio: Live,
  receipt: Receipt,
  fingerprint: Fingerprint,
  chart: Chart,
  ban: Ban,
  key: Key,
  activity: Activity,
  alert: Alert,
  folder: Folder,
  shield: ShieldCheck,
  sliders: Sliders,
  code: Code,
  users: Team,
  scroll: Scroll,
  scales: Scales,
};

const GROUPS = ["Main", "Workflows", "Compliance", "Organization"] as const;

/**
 * One navigation item: icon and label; the active item is a blue-50 pill with blue text and the
 * filled icon.
 */
export function NavItem({
  href,
  label,
  icon: ItemIcon,
  active,
  collapsed = false,
  onNavigate,
}: {
  href: NavSection["href"] | `/admin${string}`;
  label: string;
  icon: Icon;
  active: boolean;
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  return (
    <Tooltip content={label} disabled={!collapsed}>
      <Link
        href={href as NavSection["href"]}
        onClick={onNavigate}
        aria-current={active ? "page" : undefined}
        aria-label={collapsed ? label : undefined}
        className={cn(
          "flex h-10 items-center gap-3 rounded-full text-body transition-colors duration-150 ease-out",
          collapsed ? "justify-center px-0" : "px-3",
          active
            ? "bg-blue-50 font-medium text-accent"
            : "text-text-secondary hover:bg-bg-raised hover:text-text-primary",
        )}
      >
        <ItemIcon
          weight={active ? "fill" : "regular"}
          className="size-icon-md shrink-0"
          aria-hidden="true"
        />
        {collapsed ? null : <span className="truncate">{label}</span>}
      </Link>
    </Tooltip>
  );
}

/** A labelled navigation group ("Workflows"); a hairline in the collapsed rail. */
export function NavGroup({
  label,
  collapsed,
  children,
}: {
  label?: string;
  collapsed?: boolean;
  children: ReactNode;
}) {
  return (
    <div>
      {label ? (
        collapsed ? (
          <div className="mx-3 mb-2 border-t border-border-subtle" />
        ) : (
          <p className="mb-1.5 px-3 text-label text-text-muted">{label}</p>
        )
      ) : null}
      <ul className="flex flex-col gap-0.5">{children}</ul>
    </div>
  );
}

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
    <nav aria-label="Primary" className="flex flex-col gap-6">
      {GROUPS.map((g) => {
        const items = sections.filter((s) => s.group === g);
        if (items.length === 0) return null;
        return (
          <NavGroup
            key={g}
            label={g === "Main" ? undefined : g}
            collapsed={collapsed}
          >
            {items.map((s) => (
              <li key={s.href}>
                <NavItem
                  href={s.href}
                  label={s.label}
                  icon={ICONS[s.icon]}
                  active={isActiveSection(s.href, pathname)}
                  collapsed={collapsed}
                  onNavigate={onNavigate}
                />
              </li>
            ))}
          </NavGroup>
        );
      })}
    </nav>
  );
}

const menuContent =
  "z-50 w-64 rounded-md border border-border-subtle bg-bg-surface p-1.5 shadow-overlay";
export const menuItem =
  "flex w-full cursor-pointer items-center gap-2.5 rounded-sm px-3 py-2 text-left text-body outline-none data-[highlighted]:bg-bg-raised [&_svg]:size-icon-sm [&_svg]:text-text-secondary";

/**
 * The workspace switcher at the top of the sidebar: the mark, the organization and the active
 * environment. A session belongs to one organization, so the menu offers its settings.
 */
export function WorkspaceSwitcher({
  name,
  detail,
  badge,
  collapsed,
  children,
}: {
  name: string;
  detail: ReactNode;
  badge?: ReactNode;
  collapsed?: boolean;
  /** Menu items. */
  children: ReactNode;
}) {
  return (
    <M.Root>
      <M.Trigger asChild>
        <button
          type="button"
          aria-label={`${name} workspace`}
          className={cn(
            "flex w-full items-center gap-2.5 rounded-md text-left transition-colors duration-150 hover:bg-bg-raised",
            collapsed ? "justify-center p-1" : "p-2",
          )}
        >
          <Monogram name={name} className="size-9" />
          {collapsed ? null : (
            <>
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-1.5">
                  <span className="truncate text-body font-semibold">
                    {name}
                  </span>
                  {badge}
                </span>
                <span className="block truncate text-label text-text-muted">
                  {detail}
                </span>
              </span>
              <ChevronUpDown
                className="size-icon-sm shrink-0 text-text-muted"
                aria-hidden="true"
              />
            </>
          )}
        </button>
      </M.Trigger>
      <M.Portal>
        <M.Content
          align="start"
          sideOffset={6}
          className={menuContent}
          style={{ animation: "fade-in 120ms ease-out" }}
        >
          {children}
        </M.Content>
      </M.Portal>
    </M.Root>
  );
}

/** The onboarding card at the foot of the sidebar: a short title, a line and a dark button. */
export function PromoCard({
  title,
  children,
  action,
}: {
  title: string;
  children: ReactNode;
  action: ReactNode;
}) {
  return (
    <div className="bg-feature relative overflow-hidden rounded-md border border-border-subtle p-4">
      <p className="text-body font-semibold">{title}</p>
      <p className="mt-1 text-body-sm text-text-secondary">{children}</p>
      <div className="mt-3">{action}</div>
    </div>
  );
}

function EnvironmentSwitch() {
  const { environment, setEnvironment, session } = useSession();
  const locked = !session.tenant.production_enabled;
  return (
    <div
      role="radiogroup"
      aria-label="Environment"
      className="inline-flex rounded-full border border-border-strong bg-bg-surface p-0.5"
    >
      {(["test", "production"] as const).map((env) => {
        const disabled = env === "production" && locked;
        const radio = (
          <button
            key={env}
            type="button"
            role="radio"
            aria-checked={environment === env}
            aria-disabled={disabled || undefined}
            aria-describedby={disabled ? "production-locked" : undefined}
            onClick={() => (disabled ? undefined : setEnvironment(env))}
            className={cn(
              "inline-flex h-[28px] items-center gap-1 rounded-full px-3 text-label transition-colors duration-150 ease-out [&_svg]:size-3",
              environment === env
                ? env === "production"
                  ? "bg-status-warning-bg text-status-warning"
                  : "bg-ink text-white"
                : disabled
                  ? "cursor-not-allowed text-text-muted"
                  : "text-text-muted hover:text-text-primary",
            )}
          >
            {disabled ? <Lock aria-hidden="true" /> : null}
            {env === "test" ? "Test" : "Production"}
          </button>
        );
        return disabled ? (
          <Tooltip
            key={env}
            side="bottom"
            content="Opens once RiskOS approves your onboarding"
          >
            {radio}
          </Tooltip>
        ) : (
          radio
        );
      })}
      {locked ? (
        <span id="production-locked" className="sr-only">
          Production opens once RiskOS approves your compliance onboarding.
        </span>
      ) : null}
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

function UserMenu() {
  const { session } = useSession();
  const router = useRouter();
  const signOut = useSignOut();
  return (
    <M.Root>
      <M.Trigger asChild>
        <button
          type="button"
          aria-label={session.user.name}
          className="rounded-full transition-shadow duration-150 hover:shadow-[0_0_0_4px_var(--blue-50)]"
        >
          <Avatar name={session.user.name} />
        </button>
      </M.Trigger>
      <M.Portal>
        <M.Content
          align="end"
          sideOffset={8}
          className={menuContent}
          style={{ animation: "fade-in 120ms ease-out" }}
        >
          <div className="flex items-center gap-3 px-3 py-2.5">
            <Avatar name={session.user.name} />
            <div className="min-w-0">
              <p className="truncate text-body font-medium">
                {session.user.name}
              </p>
              <p className="truncate text-label text-text-muted">
                {session.user.email}
              </p>
            </div>
          </div>
          <p className="px-3 pb-2 text-label text-text-secondary">
            {session.user.role.replace("_", " ")} · {session.tenant.name}
          </p>
          <M.Separator className="my-1 border-t border-border-subtle" />
          <M.Item className={menuItem} onSelect={() => router.push("/account")}>
            <ShieldCheck /> Account security
          </M.Item>
          <M.Separator className="my-1 border-t border-border-subtle" />
          <M.Item className={menuItem} onSelect={() => void signOut()}>
            <SignOut /> Sign out
          </M.Item>
        </M.Content>
      </M.Portal>
    </M.Root>
  );
}

/** The bell: opens the alert queue, with a dot while alerts are open. */
function AlertsBell() {
  const open = useAlerts({ status: "open" });
  const count = open.data?.pages[0]?.data.length ?? 0;
  const more = !!open.data?.pages[0]?.next_cursor;
  const label = count
    ? `Alerts: ${count}${more ? "+" : ""} open`
    : "Alerts: none open";
  return (
    <Tooltip side="bottom" content={label}>
      <Button variant="secondary" size="icon-md" asChild>
        <Link href="/alerts" aria-label={label} className="relative">
          <Bell />
          {count ? (
            <span
              aria-hidden="true"
              className="absolute top-2 right-2.5 size-2 rounded-full bg-brand-red ring-2 ring-bg-surface"
            />
          ) : null}
        </Link>
      </Button>
    </Tooltip>
  );
}

/** The search field: opens the command palette (⌘K / Ctrl K). */
export function SearchField({
  onOpen,
  className,
}: {
  onOpen: () => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "flex h-control-md w-72 items-center gap-2 rounded-full border border-border-strong bg-bg-surface px-4 text-body text-text-muted transition-colors duration-150 hover:border-border-control hover:text-text-secondary",
        className,
      )}
      aria-label="Open command palette"
      aria-keyshortcuts="Control+K Meta+K"
    >
      <Search className="size-icon-sm" aria-hidden="true" />
      <span className="flex-1 text-left">Search or jump to</span>
      <Kbd>⌘K</Kbd>
    </button>
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
      const ItemIcon = ICONS[s.icon];
      return {
        id: `go-${s.href}`,
        label: s.label,
        group: "Go to",
        icon: <ItemIcon />,
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
        icon: <Alert />,
        keywords: "environment test production live",
        run: () => setEnvironment(other),
      },
      {
        id: "density",
        label:
          prefs.density === "default" ? "Use compact rows" : "Use default rows",
        group: "Actions",
        icon: <Rows />,
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
        icon: prefs.sidebarCollapsed ? <ChevronRight /> : <ChevronLeft />,
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
        icon: <SignOut />,
        run: () => void signOut(),
      },
    ];
  }, [can, environment, openHelp, prefs, router, setEnvironment, signOut]);
}

/**
 * The sidebar frame shared by the console and the admin realm (docs/design-system.md, App
 * shell): white, 248px, collapsing to a 72px rail; the workspace switcher on top, navigation,
 * then an optional footer (the promo card) and the collapse control.
 */
export function Sidebar({
  collapsed,
  onToggle,
  header,
  footer,
  children,
}: {
  collapsed: boolean;
  onToggle?: () => void;
  header: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}) {
  return (
    <aside
      className={cn(
        "sticky top-0 hidden h-dvh shrink-0 flex-col border-r border-border-subtle bg-bg-surface transition-[width] duration-200 ease-out lg:flex",
        collapsed ? "w-rail" : "w-nav",
      )}
      aria-label="Navigation"
    >
      <div className={cn("pt-4 pb-3", collapsed ? "px-3" : "px-3")}>
        {header}
      </div>
      <div
        className={cn(
          "flex-1 overflow-x-hidden overflow-y-auto py-3",
          collapsed ? "px-3" : "px-3",
        )}
      >
        {children}
      </div>
      {!collapsed && footer ? <div className="px-3 pb-3">{footer}</div> : null}
      {onToggle ? (
        <div
          className={cn(
            "flex border-t border-border-subtle py-2",
            collapsed ? "justify-center px-2" : "justify-end px-3",
          )}
        >
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
              onClick={onToggle}
            >
              {collapsed ? <ChevronRight /> : <ChevronLeft />}
            </Button>
          </Tooltip>
        </div>
      ) : null}
    </aside>
  );
}

/** The mobile drawer: the sidebar's content sliding in from the left over a scrim. */
export function MobileDrawer({
  open,
  onClose,
  header,
  footer,
  children,
}: {
  open: boolean;
  onClose: () => void;
  header: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}) {
  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-40 flex bg-ink/30 backdrop-blur-[2px] lg:hidden"
      style={{ animation: "fade-in 150ms ease-out" }}
    >
      <div
        className="m-2 flex w-nav flex-col overflow-hidden rounded-lg border border-border-subtle bg-bg-surface shadow-overlay"
        style={{ animation: "slide-in-left 180ms ease-out" }}
      >
        <div className="flex items-center gap-2 px-3 pt-3 pb-2">
          <div className="min-w-0 flex-1">{header}</div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Close navigation"
            onClick={onClose}
          >
            <Close />
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto px-3 py-3">{children}</div>
        {footer ? <div className="px-3 pb-3">{footer}</div> : null}
      </div>
      <button
        className="flex-1"
        aria-label="Close navigation"
        onClick={onClose}
      />
    </div>
  );
}

/**
 * Console layout (docs/design-system.md, App shell): the sidebar, a top bar on the canvas with
 * the environment switch, search, density, shortcuts, the alerts bell and the user menu, and the
 * page. Production gets a standing banner.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { environment, session, can } = useSession();
  const { setHelpOpen } = useShortcuts();
  const prefs = usePreferences();
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const openHelp = useCallback(() => setHelpOpen(true), [setHelpOpen]);
  const commands = useCommands(openHelp);
  const collapsed = prefs.sidebarCollapsed;
  const router = useRouter();

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

  const workspace = (isCollapsed: boolean) => (
    <WorkspaceSwitcher
      name={session.tenant.name}
      detail={
        environment === "production"
          ? "Production environment"
          : session.tenant.production_enabled
            ? "Test environment"
            : "Test · production locked"
      }
      collapsed={isCollapsed}
    >
      <div className="px-3 py-2">
        <p className="text-body font-medium">{session.tenant.name}</p>
        <p className="text-label text-text-muted">
          Signed in as {session.user.role.replace("_", " ")}
        </p>
      </div>
      <M.Separator className="my-1 border-t border-border-subtle" />
      {can("team:read") ? (
        <M.Item className={menuItem} onSelect={() => router.push("/team")}>
          <Team /> Team
        </M.Item>
      ) : null}
      {can("team:read") ? (
        <M.Item className={menuItem} onSelect={() => router.push("/sso")}>
          <Key /> Single sign-on
        </M.Item>
      ) : null}
      <M.Item className={menuItem} onSelect={() => router.push("/account")}>
        <ShieldCheck /> Account security
      </M.Item>
    </WorkspaceSwitcher>
  );

  const promo =
    can("api_keys:read") && !pathname.startsWith("/developer") ? (
      <PromoCard
        title="Send your first events"
        action={
          <Button variant="primary" size="sm" arrow asChild>
            <Link href="/developer">Developer hub</Link>
          </Button>
        }
      >
        Create an API key and get an explainable decision on every event.
      </PromoCard>
    ) : null;

  return (
    <TooltipProvider>
      <div className="flex min-h-dvh bg-bg-canvas">
        <Sidebar
          collapsed={collapsed}
          onToggle={() => setPreferences({ sidebarCollapsed: !collapsed })}
          header={workspace(collapsed)}
          footer={promo}
        >
          <NavLinks collapsed={collapsed} />
        </Sidebar>

        <MobileDrawer
          open={mobileOpen}
          onClose={() => setMobileOpen(false)}
          header={workspace(false)}
          footer={promo}
        >
          <NavLinks onNavigate={() => setMobileOpen(false)} />
        </MobileDrawer>

        <div className="flex min-w-0 flex-1 flex-col">
          {environment === "production" ? (
            <div
              role="status"
              className="flex h-9 items-center justify-center gap-2 bg-status-warning-bg px-4 lg:px-8"
            >
              <Alert
                className="size-icon-sm text-status-warning"
                aria-hidden="true"
              />
              <span className="text-label text-status-warning">
                Production: live data and live keys
              </span>
            </div>
          ) : null}
          <header className="sticky top-0 z-20 flex h-16 items-center gap-3 bg-bg-canvas/85 px-4 backdrop-blur-md lg:px-8">
            <Button
              variant="secondary"
              size="icon-md"
              className="lg:hidden"
              aria-label="Open navigation"
              onClick={() => setMobileOpen(true)}
            >
              <Menu />
            </Button>
            <EnvironmentSwitch />
            <div className="ml-auto flex items-center gap-2">
              <SearchField
                onOpen={() => setPaletteOpen(true)}
                className="hidden md:flex"
              />
              <Tooltip
                side="bottom"
                content={
                  prefs.density === "default" ? "Compact rows" : "Default rows"
                }
              >
                <Button
                  variant="secondary"
                  size="icon-md"
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
                  <Rows />
                </Button>
              </Tooltip>
              <Tooltip side="bottom" content="Keyboard shortcuts  ?">
                <Button
                  variant="secondary"
                  size="icon-md"
                  className="hidden sm:inline-flex"
                  aria-label="Keyboard shortcuts"
                  onClick={openHelp}
                >
                  <Keyboard />
                </Button>
              </Tooltip>
              {can("alerts:read") ? <AlertsBell /> : null}
              <UserMenu />
            </div>
          </header>
          <main
            key={pathname}
            className="flex-1 px-4 pt-2 pb-8 lg:px-8"
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
