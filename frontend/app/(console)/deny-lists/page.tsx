import type { Metadata } from "next";

import { DenyList } from "@/features/deny/deny";

export const metadata: Metadata = { title: "Deny lists" };

export default function Page() {
  return <DenyList />;
}
