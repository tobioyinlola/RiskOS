"use client";

import { usePathname, useRouter } from "next/navigation";
import { useCallback, type ReactNode } from "react";

import { Loading } from "@/components/ui/states";
import { SessionProvider } from "@/lib/session";

/** Onboarding needs an active session, like the console, but has its own focused frame. */
export default function OnboardingLayout({
  children,
}: {
  children: ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const toLogin = useCallback(() => {
    router.replace(`/login?next=${encodeURIComponent(pathname)}` as "/login");
  }, [router, pathname]);
  return (
    <SessionProvider
      onSignedOut={toLogin}
      fallback={
        <div className="flex min-h-dvh items-center justify-center bg-bg-surface">
          <Loading label="Checking your session" />
        </div>
      }
    >
      {children}
    </SessionProvider>
  );
}
