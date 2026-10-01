import type { Metadata } from "next";

import { StaffEnroll } from "@/features/admin/auth";

export const metadata: Metadata = { title: "Register staff passkey" };

export default function Page() {
  return <StaffEnroll />;
}
