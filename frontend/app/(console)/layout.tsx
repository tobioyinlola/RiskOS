"use client";

import { usePathname, useRouter } from "next/navigation";
import { useCallback, type ReactNode } from "react";

import { Loading } from "@/components/instrument/states";
import { AppShell } from "@/components/shell/app-shell";
import { ReauthProvider } from "@/components/shell/reauth";
import { ShortcutsProvider } from "@/components/shell/shortcuts";
import { SessionProvider } from "@/lib/session";

/** Every console page needs an active session; otherwise the user signs in first. */
export default function ConsoleLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const toLogin = useCallback(() => {
    const next =
      pathname && pathname !== "/"
        ? `?next=${encodeURIComponent(pathname)}`
        : "";
    router.replace(`/login${next}` as "/login");
  }, [router, pathname]);
  return (
    <SessionProvider
      onSignedOut={toLogin}
      fallback={
        <div className="flex min-h-dvh items-center justify-center">
          <Loading label="Checking your session" />
        </div>
      }
    >
      <ShortcutsProvider>
        <ReauthProvider>
          <AppShell>{children}</AppShell>
        </ReauthProvider>
      </ShortcutsProvider>
    </SessionProvider>
  );
}
