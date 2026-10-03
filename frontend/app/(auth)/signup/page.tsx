import type { Metadata } from "next";
import { Suspense } from "react";

import { Signup } from "@/features/auth/signup";

export const metadata: Metadata = { title: "Create your workspace" };

export default function Page() {
  return (
    <Suspense>
      <Signup />
    </Suspense>
  );
}
