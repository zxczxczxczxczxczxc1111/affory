import type { CSSProperties, ReactNode } from "react";
import { VKLADKI, nazvanieVkladki, type Vkladka } from "./vkladki";
import sphere from "../assets/affory-sphere.png";

// Window frame: our own title bar (the window is frameless, §8.3) and the
// four tabs. Pure over props like every screen; minimise and close are calls
// UP to App, which owns the bridge. This file never imports the runtime.

export interface KarkasProps {
  vkladka: Vkladka;
  naVkladku: (v: Vkladka) => void;
  naSvernut: () => void;
  naZakryt: () => void;
  /** First run (§9.2): tabs are disabled until the service exists. The
   *  window controls stay live so the window can still be closed. */
  zablokirovany?: boolean;
  children: ReactNode;
}

// Wails reads these custom properties to decide what drags the window.
// Inline, not in a class: the test asserts on the computed inline value, and
// a class would silently stop working the day someone renames it.
const TASHCHIT: CSSProperties = { ["--wails-draggable" as string]: "drag" } as CSSProperties;
const NE_TASHCHIT: CSSProperties = { ["--wails-draggable" as string]: "no-drag" } as CSSProperties;

export function Karkas({ vkladka, naVkladku, naSvernut, naZakryt, zablokirovany = false, children }: KarkasProps) {
  return (
    <div className="flex h-screen flex-col">
      <header
        data-testid="polosa"
        style={TASHCHIT}
        className="af-titlebar flex h-polosa shrink-0 items-center justify-between border-b border-border bg-rail pl-4 select-none"
      >
        <div className="af-title-left flex items-center gap-6">
          <span className="af-brand flex items-center gap-2 text-[17px] font-semibold tracking-tight"><img src={sphere} alt="" className="h-6 w-6" />Affory</span>
          <nav role="tablist" aria-label="Разделы" className="flex h-polosa items-stretch">
            {VKLADKI.filter(v => v !== "servery").map((v) => {
              const aktivna = v === vkladka || (v === "podklyuchenie" && vkladka === "servery");
              return (
                <button
                  key={v}
                  type="button"
                  role="tab"
                  aria-selected={aktivna}
                  style={NE_TASHCHIT}
                  disabled={zablokirovany}
                  onClick={() => naVkladku(v)}
                  className={
                    aktivna
                      // Ink, never fill: a fill-only tab has no edge on black.
                      ? "text-accent-ink border-b-2 border-accent-ink px-3 text-sm font-medium disabled:opacity-40"
                      : "text-fg-muted hover:text-foreground border-b-2 border-transparent px-3 text-sm font-medium disabled:opacity-40 disabled:hover:text-fg-muted"
                  }
                >
                  {nazvanieVkladki(v)}
                </button>
              );
            })}
          </nav>
        </div>
        <div className="af-window-controls flex h-polosa items-stretch">
          <button
            type="button"
            data-testid="svernut"
            aria-label="Свернуть"
            style={NE_TASHCHIT}
            onClick={naSvernut}
            className="text-fg-muted hover:bg-fill-hover hover:text-foreground w-12 text-base"
          >
            &#x2013;
          </button>
          <button
            type="button"
            data-testid="zakryt"
            aria-label="Закрыть"
            style={NE_TASHCHIT}
            onClick={naZakryt}
            className="text-fg-muted hover:bg-danger hover:text-background w-12 text-base"
          >
            &#x00D7;
          </button>
        </div>
      </header>
      <div className="af-viewport min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}
