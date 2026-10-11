import type { Metadata } from "next";
import { Suspense } from "react";

import { StaffSSOCallback } from "@/features/admin/auth";

export const metadata: Metadata = { title: "Staff single sign-on" };

export default function Page() {
  return (
    <Suspense>
      <StaffSSOCallback />
    </Suspense>
  );
}
