import type { Metadata } from "next";
import { Suspense } from "react";

import { SSOCallback } from "@/features/auth/login";

export const metadata: Metadata = { title: "Signing in" };

export default function Page() {
  return (
    <Suspense>
      <SSOCallback />
    </Suspense>
  );
}
