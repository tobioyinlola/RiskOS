import type { Metadata } from "next";

import { StaffAuditPage } from "@/features/admin/pages";

export const metadata: Metadata = { title: "Staff: audit log" };

export default function Page() {
  return <StaffAuditPage />;
}
