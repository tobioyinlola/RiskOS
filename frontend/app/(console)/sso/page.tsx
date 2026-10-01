import type { Metadata } from "next";

import { SingleSignOn } from "@/features/org/sso";

export const metadata: Metadata = { title: "Single sign-on" };

export default function Page() {
  return <SingleSignOn />;
}
