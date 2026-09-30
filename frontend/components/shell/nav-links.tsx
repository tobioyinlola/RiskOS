"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { isActiveSection, navSections } from "@/lib/navigation";

interface NavLinksProps {
  onNavigate?: () => void;
}

export function NavLinks({ onNavigate }: NavLinksProps) {
  const pathname = usePathname();

  return (
    <ul className="flex flex-col gap-0.5">
      {navSections.map((section) => {
        const active = isActiveSection(section.href, pathname);
        return (
          <li key={section.href}>
            <Link
              href={section.href}
              onClick={onNavigate}
              aria-current={active ? "page" : undefined}
              className={cn(
                "block rounded-md px-3 py-2 text-sm transition-colors",
                "hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                "outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                active
                  ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                  : "text-sidebar-foreground/80",
              )}
            >
              {section.label}
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
