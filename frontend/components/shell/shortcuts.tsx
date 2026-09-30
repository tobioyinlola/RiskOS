"use client";

import { useRouter } from "next/navigation";
import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

export interface Binding {
  keys: string;
  label: string;
}

interface ShortcutsValue {
  helpOpen: boolean;
  setHelpOpen: (open: boolean) => void;
  bindings: Binding[];
  register: (bindings: Binding[]) => () => void;
}

const ShortcutsContext = createContext<ShortcutsValue | null>(null);

export function useShortcuts(): ShortcutsValue {
  const ctx = useContext(ShortcutsContext);
  if (!ctx) throw new Error("useShortcuts outside ShortcutsProvider");
  return ctx;
}

/** True when a key press belongs to a text field and must not trigger shortcuts. */
export function isTyping(e: KeyboardEvent): boolean {
  const t = e.target as HTMLElement | null;
  if (!t) return false;
  return (
    t.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName)
  );
}

/**
 * Global shortcuts: `?` opens the help, `g` then a letter navigates. Pages register their own
 * bindings (shown in the help) and handle their keys themselves.
 */
export function ShortcutsProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [helpOpen, setHelpOpen] = useState(false);
  const [bindings, setBindings] = useState<Binding[]>([]);
  const lastG = useRef(0);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e) || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "?") {
        e.preventDefault();
        setHelpOpen(true);
        return;
      }
      if (e.key === "g") {
        lastG.current = Date.now();
        return;
      }
      if (Date.now() - lastG.current < 800) {
        const target = {
          a: "/alerts",
          e: "/events",
          c: "/cases",
          r: "/rules",
          o: "/",
        }[e.key];
        if (target) {
          e.preventDefault();
          lastG.current = 0;
          router.push(target as "/alerts");
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);

  const register = (b: Binding[]) => {
    setBindings(b);
    return () => setBindings([]);
  };

  return (
    <ShortcutsContext.Provider
      value={{ helpOpen, setHelpOpen, bindings, register }}
    >
      {children}
    </ShortcutsContext.Provider>
  );
}

/** Registers a page's shortcuts for the help overlay while the page is mounted. */
export function usePageShortcuts(bindings: Binding[]) {
  const { register } = useShortcuts();
  const key = bindings.map((b) => b.keys + b.label).join("|");
  useEffect(() => register(bindings), [key]); // eslint-disable-line react-hooks/exhaustive-deps
}
