import type { Metadata } from "next";

import { OnboardingReviewsPage } from "@/features/admin/onboarding";

export const metadata: Metadata = { title: "Staff: onboarding reviews" };

export default function Page() {
  return <OnboardingReviewsPage />;
}
