import type { Metadata } from "next";

import { StaffRosterPage } from "@/features/admin/roster";

export const metadata: Metadata = { title: "Staff: accounts" };

export default function Page() {
  return <StaffRosterPage />;
}
