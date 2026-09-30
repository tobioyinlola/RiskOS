import type { Metadata } from "next";

import { AcceptInvite } from "@/features/auth/login";

export const metadata: Metadata = { title: "Accept invite" };

export default function Page() {
  return <AcceptInvite />;
}
