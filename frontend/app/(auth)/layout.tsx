import type { ReactNode } from "react";

import { AuthFrame } from "@/components/shell/auth-frame";

/** Sign-in pages: one card on the landing wash. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return <AuthFrame tag="Risk console">{children}</AuthFrame>;
}
