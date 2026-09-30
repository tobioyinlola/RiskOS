import type { Metadata } from "next";

import { AccountSecurity } from "@/features/org/org";

export const metadata: Metadata = { title: "Account security" };

export default function Page() {
  return <AccountSecurity />;
}
