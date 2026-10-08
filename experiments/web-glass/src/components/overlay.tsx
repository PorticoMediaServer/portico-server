import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Check, X } from 'lucide-react';
import { create } from 'zustand';
import { useIsMobile, useLockBody } from '../lib/hooks';
import { cls } from './ui';

// ---------- Popover ----------

export function Popover({
  anchor,
  open,
  onClose,
  children,
  align = 'end',
  width,
  className,
  label,
}: {
  anchor: HTMLElement | null;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  align?: 'start' | 'end' | 'center';
  width?: number;
  className?: string;
  label?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number; origin: string; maxH: number } | null>(null);

  useLayoutEffect(() => {
    if (!open || !anchor) return;
    const place = () => {
      const r = anchor.getBoundingClientRect();
      const el = ref.current;
      const w = el?.offsetWidth ?? width ?? 260;
      const h = el?.offsetHeight ?? 300;
      let left = align === 'end' ? r.right - w : align === 'center' ? r.left + r.width / 2 - w / 2 : r.left;
      left = Math.max(12, Math.min(left, window.innerWidth - w - 12));
      let top = r.bottom + 10;
      let origin = align === 'end' ? 'top right' : align === 'center' ? 'top center' : 'top left';
      const below = window.innerHeight - r.bottom - 22;
      let maxH = below;
      if (h > below && r.top > below) {
        top = Math.max(12, r.top - h - 10);
        origin = origin.replace('top', 'bottom');
        maxH = r.top - 22;
      }
      setPos({ top, left, origin, maxH: Math.max(200, maxH) });
    };
    place();
    const raf = requestAnimationFrame(place);
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    };
  }, [open, anchor, align, width]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node;
      if (ref.current?.contains(t) || anchor?.contains(t)) return;
      onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
        anchor?.focus();
      }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        const items = Array.from(ref.current?.querySelectorAll<HTMLElement>('.menu-item:not(:disabled)') ?? []);
        if (!items.length) return;
        e.preventDefault();
        const i = items.indexOf(document.activeElement as HTMLElement);
        const next = e.key === 'ArrowDown' ? (i + 1) % items.length : (i - 1 + items.length) % items.length;
        items[next].focus();
      }
    };
    document.addEventListener('pointerdown', onDown, true);
    document.addEventListener('keydown', onKey, true);
    return () => {
      document.removeEventListener('pointerdown', onDown, true);
      document.removeEventListener('keydown', onKey, true);
    };
  }, [open, onClose, anchor]);

  if (!open) return null;
  return createPortal(
    <div
      ref={ref}
      role="dialog"
      aria-label={label}
      className={cls('popover glass glass-thick', className)}
      style={{ top: pos?.top ?? -9999, left: pos?.left ?? -9999, width, transformOrigin: pos?.origin, maxHeight: pos?.maxH, overflowY: 'auto' }}
    >
      {children}
    </div>,
    document.body,
  );
}

// ---------- Menu (popover on desktop, action sheet on phones) ----------

export interface MenuItem {
  label?: string;
  icon?: ReactNode;
  onSelect?: () => void;
  checked?: boolean;
  danger?: boolean;
  hint?: string;
  disabled?: boolean;
  separator?: boolean;
  heading?: string;
}

export function Menu({ anchor, open, onClose, items, title, align = 'end', width = 260 }: { anchor: HTMLElement | null; open: boolean; onClose: () => void; items: MenuItem[]; title?: string; align?: 'start' | 'end' | 'center'; width?: number }) {
  const mobile = useIsMobile();
  const list = (
    <div role="menu" aria-label={title}>
      {items.map((it, i) =>
        it.separator ? (
          <div key={i} className="menu-sep" />
        ) : it.heading ? (
          <div key={i} className="menu-label">
            {it.heading}
          </div>
        ) : (
          <button
            key={i}
            role="menuitem"
            className={cls('menu-item', it.danger && 'danger')}
            disabled={it.disabled}
            onClick={() => {
              onClose();
              it.onSelect?.();
            }}
          >
            {it.icon}
            <span>{it.label}</span>
            {it.checked && <Check className="check" />}
            {it.hint && <span className="hint">{it.hint}</span>}
          </button>
        ),
      )}
    </div>
  );
  if (mobile)
    return (
      <Sheet open={open} onClose={onClose} title={title} compact>
        <div className="action-sheet">{list}</div>
      </Sheet>
    );
  return (
    <Popover anchor={anchor} open={open} onClose={onClose} align={align} width={width} label={title}>
      {list}
    </Popover>
  );
}

export function useMenu() {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const open = useCallback((e: React.MouseEvent<HTMLElement>) => setAnchor((a) => (a ? null : e.currentTarget)), []);
  const close = useCallback(() => setAnchor(null), []);
  return { anchor, isOpen: !!anchor, open, close };
}

// ---------- Sheet (bottom sheet on phones, centered panel on larger screens) ----------

export function Sheet({
  open,
  onClose,
  title,
  children,
  footer,
  size = 'md',
  compact,
  className,
  headerAction,
}: {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  compact?: boolean;
  className?: string;
  headerAction?: ReactNode;
}) {
  const mobile = useIsMobile();
  const [mounted, setMounted] = useState(open);
  const [closing, setClosing] = useState(false);
  const panel = useRef<HTMLDivElement>(null);
  const drag = useRef<{ y0: number; dy: number; t0: number } | null>(null);
  useLockBody(open);

  useEffect(() => {
    if (open) {
      setMounted(true);
      setClosing(false);
    } else if (mounted) {
      setClosing(true);
      const t = setTimeout(() => {
        setMounted(false);
        setClosing(false);
      }, 280);
      return () => clearTimeout(t);
    }
  }, [open, mounted]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener('keydown', onKey);
    const prev = document.activeElement as HTMLElement | null;
    requestAnimationFrame(() => (panel.current?.querySelector<HTMLElement>('[autofocus], input:not([type=checkbox]), textarea') ?? panel.current)?.focus({ preventScroll: true }));
    return () => {
      document.removeEventListener('keydown', onKey);
      prev?.focus?.({ preventScroll: true });
    };
  }, [open, onClose]);

  const onPointerDown = (e: React.PointerEvent) => {
    if (!mobile) return;
    const scroller = panel.current?.querySelector('.sheet-body');
    if (scroller && scroller.scrollTop > 0 && !(e.target as HTMLElement).closest('.sheet-head')) return;
    drag.current = { y0: e.clientY, dy: 0, t0: performance.now() };
  };
  const onPointerMove = (e: React.PointerEvent) => {
    if (!drag.current || !panel.current) return;
    const dy = Math.max(0, e.clientY - drag.current.y0);
    drag.current.dy = dy;
    if (dy > 4) {
      panel.current.style.transition = 'none';
      panel.current.style.transform = `translateY(${dy}px)`;
    }
  };
  const onPointerUp = () => {
    const d = drag.current;
    drag.current = null;
    if (!d || !panel.current) return;
    panel.current.style.transition = '';
    const velocity = d.dy / Math.max(1, performance.now() - d.t0);
    if (d.dy > 120 || velocity > 0.6) {
      panel.current.style.transform = `translateY(100%)`;
      onClose();
    } else panel.current.style.transform = '';
  };

  if (!mounted) return null;
  return createPortal(
    <div className={cls('sheet-root', mobile ? 'is-mobile' : 'is-desktop', closing && 'closing')}>
      <div className="sheet-scrim" onClick={onClose} />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
        className={cls('sheet glass glass-thick', `sheet-${size}`, compact && 'compact', className)}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        {mobile && <div className="sheet-grabber" aria-hidden="true" />}
        {(title || !compact) && (
          <div className="sheet-head">
            <div className="sheet-title">{title}</div>
            {headerAction}
            <button className="icon-btn sm plain sheet-close" aria-label="Close" onClick={onClose}>
              <X />
            </button>
          </div>
        )}
        <div className="sheet-body">{children}</div>
        {footer && <div className="sheet-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

// ---------- Confirm dialog ----------

export function Confirm({ open, onClose, onConfirm, title, body, confirmLabel = 'Confirm', danger }: { open: boolean; onClose: () => void; onConfirm: () => void; title: string; body?: ReactNode; confirmLabel?: string; danger?: boolean }) {
  return (
    <Sheet open={open} onClose={onClose} size="sm" compact title={title}>
      <div className="confirm">
        {body && <p className="t-body">{body}</p>}
        <div className="confirm-actions">
          <button className="btn btn-plain" onClick={onClose}>
            Cancel
          </button>
          <button
            className={cls('btn', danger ? 'btn-danger-solid' : 'btn-accent')}
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </Sheet>
  );
}

// ---------- Toasts ----------

interface Toast {
  id: number;
  text: string;
  icon?: ReactNode;
  action?: { label: string; run: () => void };
}
interface ToastState {
  toasts: Toast[];
  push: (t: Omit<Toast, 'id'>) => void;
  dismiss: (id: number) => void;
}
let tid = 0;
export const useToasts = create<ToastState>((set, get) => ({
  toasts: [],
  push: (t) => {
    const id = ++tid;
    set({ toasts: [...get().toasts.slice(-2), { ...t, id }] });
    setTimeout(() => get().dismiss(id), 3600);
  },
  dismiss: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));
export const toast = (text: string, icon?: ReactNode, action?: Toast['action']) => useToasts.getState().push({ text, icon, action });

export function Toaster() {
  const toasts = useToasts((s) => s.toasts);
  const dismiss = useToasts((s) => s.dismiss);
  return (
    <div className="toasts" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className="toast glass glass-thick">
          {t.icon}
          <span>{t.text}</span>
          {t.action && (
            <button
              onClick={() => {
                t.action!.run();
                dismiss(t.id);
              }}
            >
              {t.action.label}
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
