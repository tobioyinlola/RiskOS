import type { ReactNode } from "react";

import { AuthFrame } from "@/components/shell/auth-frame";

/** Staff sign-in pages: one card on the landing wash, tagged Admin. */
export default function StaffAuthLayout({ children }: { children: ReactNode }) {
  return (
    <AuthFrame tag="Admin · internal" title="Staff access," accent="audited.">
      {children}
    </AuthFrame>
  );
}
