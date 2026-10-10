"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { DropdownMenu as M } from "radix-ui";
import { usePathname, useRouter } from "next/navigation";
import { createContext, useContext, useEffect, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Loading } from "@/components/ui/states";
import {
  Organization,
  Report,
  Scroll,
  ShieldCheck,
  SignOut,
  UserAdmin,
} from "@/components/ui/icons";
import { pillTabClass } from "@/components/ui/misc";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Wordmark } from "@/components/ui/wordmark";
import {
  menuItem,
  NavGroup,
  NavItem,
  PromoCard,
  Sidebar,
  WorkspaceSwitcher,
} from "@/components/shell/app-shell";
import {
  adminApi,
  setStaffCsrfToken,
  type AdminSchemas,
} from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatTimestamp } from "@/lib/format";

type StaffSession = AdminSchemas["StaffSession"];

const StaffContext = createContext<StaffSession | null>(null);

export function useStaff(): StaffSession {
  const s = useContext(StaffContext);
  if (!s) throw new Error("useStaff outside StaffShell");
  return s;
}

const NAV = [
  { href: "/admin", label: "Overview", icon: ShieldCheck },
  { href: "/admin/tenants", label: "Tenants", icon: Organization },
  { href: "/admin/onboarding", label: "Onboarding reviews", icon: Report },
  { href: "/admin/support", label: "Support access", icon: UserAdmin },
  { href: "/admin/audit", label: "Staff audit log", icon: Scroll },
] as const;

/** The "Admin" badge beside the workspace name, so the two realms never look alike. */
function AdminBadge() {
  return (
    <span className="rounded-full bg-ink px-2 py-px text-label text-white">
      Admin
    </span>
  );
}

/** The staff console: separate from the customer console, signed in with a staff passkey. */
export function StaffShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["staff", "session"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/auth/session")),
    retry: false,
    refetchInterval: 60_000,
  });
  const unauthenticated = q.error instanceof ApiError && q.error.status === 401;

  useEffect(() => {
    if (q.data) setStaffCsrfToken(q.data.csrf_token);
  }, [q.data]);
  useEffect(() => {
    if (unauthenticated) router.replace("/admin/login");
  }, [unauthenticated, router]);

  const signOut = async () => {
    await adminApi().POST("/api/admin/v1/auth/logout");
    setStaffCsrfToken("");
    qc.clear();
    router.replace("/admin/login");
  };

  if (!q.data) {
    return (
      <main className="mx-auto max-w-5xl px-4 py-7">
        <Loading label="Checking your staff session" />
      </main>
    );
  }
  const isActive = (href: string) =>
    href === "/admin" ? pathname === "/admin" : pathname.startsWith(href);
  const staff = q.data.staff;
  return (
    <StaffContext.Provider value={q.data}>
      <TooltipProvider>
        <div className="flex min-h-dvh bg-bg-canvas">
          <Sidebar
            collapsed={false}
            header={
              <WorkspaceSwitcher
                name="RiskOS"
                badge={<AdminBadge />}
                detail="Internal staff console"
              >
                <div className="px-3 py-2">
                  <p className="text-body font-medium">{staff.name}</p>
                  <p className="text-label text-text-muted">
                    Session ends {formatTimestamp(q.data.expires_at)}
                  </p>
                </div>
                <M.Separator className="my-1 border-t border-border-subtle" />
                <M.Item className={menuItem} onSelect={() => void signOut()}>
                  <SignOut /> Sign out
                </M.Item>
              </WorkspaceSwitcher>
            }
            footer={
              <PromoCard
                title="Four-eyes support"
                action={
                  <Button variant="primary" size="sm" arrow asChild>
                    <Link href="/admin/support">Review requests</Link>
                  </Button>
                }
              >
                Tenant access needs a second staff member and ends on its own.
              </PromoCard>
            }
          >
            <nav aria-label="Staff console">
              <NavGroup>
                {NAV.map((n) => (
                  <li key={n.href}>
                    <NavItem
                      href={n.href}
                      label={n.label}
                      icon={n.icon}
                      active={isActive(n.href)}
                    />
                  </li>
                ))}
              </NavGroup>
            </nav>
          </Sidebar>

          <div className="flex min-w-0 flex-1 flex-col">
            <header className="sticky top-0 z-20 bg-bg-canvas/85 backdrop-blur-md">
              <div className="flex h-16 items-center gap-3 px-4 lg:px-8">
                <span className="flex items-center gap-2 lg:hidden">
                  <Wordmark className="text-heading-3" />
                  <AdminBadge />
                </span>
                <div className="ml-auto flex items-center gap-3">
                  <span className="hidden text-label text-text-muted sm:inline">
                    {staff.name} · session ends{" "}
                    {formatTimestamp(q.data.expires_at)}
                  </span>
                  <Button
                    size="sm"
                    icon={<SignOut />}
                    onClick={() => void signOut()}
                  >
                    Sign out
                  </Button>
                </div>
              </div>
              <nav
                aria-label="Staff console sections"
                className="flex gap-2 overflow-x-auto px-4 pb-3 lg:hidden"
              >
                {NAV.map((n) => (
                  <Link
                    key={n.href}
                    href={n.href}
                    aria-current={isActive(n.href) ? "page" : undefined}
                    className={pillTabClass(isActive(n.href))}
                  >
                    {n.label}
                  </Link>
                ))}
              </nav>
            </header>
            <main className="flex-1 px-4 pt-2 pb-8 lg:px-8">{children}</main>
          </div>
        </div>
      </TooltipProvider>
    </StaffContext.Provider>
  );
}
