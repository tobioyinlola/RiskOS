"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { cn } from "cn";
import { LogOut } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, useContext, useEffect, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Loading } from "@/components/ui/states";
import { Wordmark } from "@/components/ui/wordmark";
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
  { href: "/admin", label: "Security" },
  { href: "/admin/support", label: "Support access" },
  { href: "/admin/audit", label: "Staff audit log" },
] as const;

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
  return (
    <StaffContext.Provider value={q.data}>
      <div className="min-h-dvh bg-bg-canvas">
        <header className="border-b border-border-subtle bg-bg-surface">
          <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-4 px-4 py-3">
            <Wordmark />
            <span className="rounded-sm bg-risk-high-bg px-2 text-overline text-text-primary uppercase">
              Staff
            </span>
            <nav className="flex flex-wrap gap-1" aria-label="Staff console">
              {NAV.map((n) => {
                const active =
                  n.href === "/admin"
                    ? pathname === "/admin"
                    : pathname.startsWith(n.href);
                return (
                  <Link
                    key={n.href}
                    href={n.href}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "rounded-sm px-3 py-1 text-body-sm",
                      active
                        ? "bg-bg-raised text-text-primary"
                        : "text-text-secondary hover:text-text-primary",
                    )}
                  >
                    {n.label}
                  </Link>
                );
              })}
            </nav>
            <div className="ml-auto flex items-center gap-3">
              <span className="text-label text-text-muted">
                {q.data.staff.name} · session ends{" "}
                {formatTimestamp(q.data.expires_at)}
              </span>
              <Button
                size="sm"
                variant="ghost"
                icon={<LogOut />}
                onClick={() => void signOut()}
              >
                Sign out
              </Button>
            </div>
          </div>
        </header>
        <main className="mx-auto max-w-6xl px-4 py-6">{children}</main>
      </div>
    </StaffContext.Provider>
  );
}
