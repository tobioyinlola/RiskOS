import type { Metadata } from "next";

import { StaffLogin } from "@/features/admin/auth";

export const metadata: Metadata = { title: "Staff sign-in" };

export default function Page() {
  return <StaffLogin />;
}
