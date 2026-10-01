"use client";

import { cn } from "cn";
import { CornerDownLeft, Search } from "lucide-react";
import { Dialog as D } from "radix-ui";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { Kbd } from "@/components/ui/kbd";

export interface Command {
  id: string;
  label: string;
  group: string;
  icon?: ReactNode;
  keywords?: string;
  shortcut?: string;
  run: () => void;
}

/**
 * The command palette (Ctrl+K / ⌘K): jump to any section or run a console action without
 * leaving the keyboard. Type to filter, arrows to move, Enter to run, Escape to close.
 */
export function CommandPalette({
  open,
  onOpenChange,
  commands,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commands: Command[];
}) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLUListElement>(null);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    const words = q.split(/\s+/);
    return commands.filter((c) => {
      const hay = `${c.label} ${c.group} ${c.keywords ?? ""}`.toLowerCase();
      return words.every((w) => hay.includes(w));
    });
  }, [commands, query]);

  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const run = (c: Command | undefined) => {
    if (!c) return;
    onOpenChange(false);
    c.run();
  };

  return (
    <D.Root
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          setQuery("");
          setActive(0);
        }
        onOpenChange(o);
      }}
    >
      <D.Portal>
        <D.Overlay
          className="fixed inset-0 z-50 bg-bg-canvas/70"
          style={{ animation: "fade-in 120ms ease-out" }}
        />
        <D.Content
          className="fixed top-[12vh] left-1/2 z-50 flex max-h-[70vh] w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 flex-col overflow-hidden rounded-lg border border-border-subtle bg-bg-surface shadow-overlay outline-none"
          style={{ animation: "fade-in 120ms ease-out" }}
          aria-describedby={undefined}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setActive((i) => Math.min(results.length - 1, i + 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setActive((i) => Math.max(0, i - 1));
            } else if (e.key === "Enter") {
              e.preventDefault();
              run(results[active]);
            }
          }}
        >
          <D.Title className="sr-only">Command palette</D.Title>
          <div className="flex items-center gap-2 border-b border-border-subtle px-4">
            <Search
              className="size-icon-sm shrink-0 stroke-[1.5] text-text-muted"
              aria-hidden="true"
            />
            <input
              autoFocus
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setActive(0);
              }}
              placeholder="Go to a section or run an action"
              aria-label="Search commands"
              role="combobox"
              aria-expanded="true"
              aria-controls="command-results"
              aria-activedescendant={
                results[active] ? `command-${results[active].id}` : undefined
              }
              className="h-12 flex-1 bg-transparent text-body text-text-primary outline-none placeholder:text-text-muted"
            />
            <Kbd>Esc</Kbd>
          </div>
          <ul
            id="command-results"
            role="listbox"
            ref={listRef}
            className="flex-1 overflow-y-auto p-2"
          >
            {results.length === 0 ? (
              <li className="px-3 py-5 text-body-sm text-text-muted">
                Nothing matches “{query}”.
              </li>
            ) : null}
            {results.map((c, i) => {
              const header =
                i === 0 || results[i - 1]?.group !== c.group ? c.group : null;
              return (
                <li key={c.id} role="presentation">
                  {header ? (
                    <p className="px-3 pt-3 pb-1 text-overline text-text-secondary uppercase">
                      {header}
                    </p>
                  ) : null}
                  <div
                    id={`command-${c.id}`}
                    role="option"
                    aria-selected={i === active}
                    data-index={i}
                    onMouseMove={() => setActive(i)}
                    onClick={() => run(c)}
                    className={cn(
                      "flex h-control-md cursor-pointer items-center gap-3 rounded-md px-3 text-body",
                      i === active
                        ? "bg-accent-subtle text-text-primary"
                        : "text-text-secondary",
                    )}
                  >
                    <span className="flex size-icon-sm items-center justify-center [&_svg]:size-icon-sm [&_svg]:stroke-[1.5]">
                      {c.icon}
                    </span>
                    <span className="flex-1 truncate">{c.label}</span>
                    {c.shortcut ? (
                      <span className="flex gap-1">
                        {c.shortcut.split(" ").map((k) => (
                          <Kbd key={k}>{k}</Kbd>
                        ))}
                      </span>
                    ) : null}
                    {i === active ? (
                      <CornerDownLeft
                        className="size-icon-sm stroke-[1.5] text-text-muted"
                        aria-hidden="true"
                      />
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}
