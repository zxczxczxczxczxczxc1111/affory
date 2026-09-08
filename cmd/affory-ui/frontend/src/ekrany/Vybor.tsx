import { useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";

export function CheckIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m5 12 4 4L19 6" /></svg>;
}

export function Vybor<T extends string>({ value, label, options, disabled = false, onChange }: {
  value: T;
  label: string;
  options: readonly { value: T; label: string }[];
  disabled?: boolean;
  onChange: (value: T) => void;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const search = useRef({ text: "", time: 0 });
  const selected = Math.max(0, options.findIndex(option => option.value === value));
  const [expanded, setExpanded] = useState(false);
  const [active, setActive] = useState(selected);
  const [position, setPosition] = useState<CSSProperties>({ visibility: "hidden" });
  const open = expanded && !disabled && options.length > 0;
  const current = Math.min(active, options.length - 1);
  const show = () => { setActive(selected); setExpanded(true); search.current.text = ""; };
  const choose = (index: number) => {
    const option = options[index];
    if (disabled || !option) return;
    setExpanded(false);
    onChange(option.value);
    trigger.current?.focus();
  };
  useEffect(() => { if (disabled) setExpanded(false); }, [disabled]);
  // A menu belongs above clipped cards, not inside their tiny CSS prison.
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = trigger.current?.getBoundingClientRect();
      if (!rect) return;
      const below = window.innerHeight - rect.bottom - 14;
      const above = rect.top - 14;
      const desired = Math.min(options.length * 40 + 12, 280);
      const up = below < desired && above > below;
      const width = Math.min(Math.max(rect.width, 200), window.innerWidth - 24);
      setPosition({ position: "fixed", width, left: Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)),
        top: up ? undefined : rect.bottom + 6, bottom: up ? window.innerHeight - rect.top + 6 : undefined,
        maxHeight: Math.max(40, Math.min(280, up ? above : below)), visibility: "visible" });
    };
    place();
    const onScroll = (event: Event) => { if (!menu.current?.contains(event.target as Node)) place(); };
    const outside = (event: PointerEvent) => {
      if (!trigger.current?.contains(event.target as Node) && !menu.current?.contains(event.target as Node)) setExpanded(false);
    };
    window.addEventListener("resize", place);
    document.addEventListener("scroll", onScroll, true);
    document.addEventListener("pointerdown", outside, true);
    return () => {
      window.removeEventListener("resize", place);
      document.removeEventListener("scroll", onScroll, true);
      document.removeEventListener("pointerdown", outside, true);
    };
  }, [open, options.length]);
  useEffect(() => {
    if (open) menu.current?.children[current]?.scrollIntoView?.({ block: "nearest" });
  }, [open, current]);
  const keyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (disabled || !options.length) return;
    const { key } = event;
    if (key === "Tab") { setExpanded(false); return; }
    if (key === "Escape") { if (open) { event.preventDefault(); event.stopPropagation(); setExpanded(false); } return; }
    if (key === "Enter" || key === " ") { event.preventDefault(); if (open) choose(current); else show(); return; }
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(key)) {
      event.preventDefault();
      if (!open) show();
      if (key === "Home") setActive(0);
      else if (key === "End") setActive(options.length - 1);
      else if (open) setActive((current + (key === "ArrowDown" ? 1 : -1) + options.length) % options.length);
      return;
    }
    if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      const now = Date.now();
      const char = key.toLocaleLowerCase("ru-RU");
      const previous = now - search.current.time < 700 ? search.current.text : "";
      const text = previous === char ? char : previous + char;
      search.current = { text, time: now };
      if (!open) setExpanded(true);
      const indices = options.map((_, index) => (current + index + 1) % options.length);
      const next = indices.find(index => options[index].label.toLocaleLowerCase("ru-RU").startsWith(text));
      if (next !== undefined) setActive(next);
    }
  };
  return <span className="af-picker">
    <button ref={trigger} type="button" role="combobox" className="af-select" aria-label={label}
      aria-haspopup="listbox" aria-controls={open ? id : undefined} aria-expanded={open}
      aria-activedescendant={open ? `${id}-${current}` : undefined} disabled={disabled || !options.length}
      onClick={() => open ? setExpanded(false) : show()} onKeyDown={keyDown}
      onBlur={() => setExpanded(false)}>
      <span>{options.find(option => option.value === value)?.label ?? "Выберите…"}</span>
      <svg className="af-chevron" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m6 8 4 4 4-4" /></svg>
    </button>
    {open && createPortal(<div className="affory-desktop af-picker-layer">
      <div ref={menu} id={id} role="listbox" aria-label={label} className="af-select-menu" style={position}>
        {options.map((option, index) => <div key={option.value} id={`${id}-${index}`} role="option"
          aria-selected={option.value === value} data-active={index === current}
          className="af-select-option" onPointerMove={() => setActive(index)}
          onMouseDown={event => event.preventDefault()} onClick={() => choose(index)}>
          <span>{option.label}</span>{option.value === value && <CheckIcon />}
        </div>)}
      </div>
    </div>, document.body)}
  </span>;
}
