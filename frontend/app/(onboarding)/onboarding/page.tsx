import type { Metadata } from "next";

import { Onboarding } from "@/features/onboarding/onboarding";

export const metadata: Metadata = { title: "Compliance onboarding" };

export default function Page() {
  return <Onboarding />;
}
