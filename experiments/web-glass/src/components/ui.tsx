import { forwardRef, useLayoutEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { AlertTriangle, RotateCw } from 'lucide-react';
import { Link } from 'react-router';
import { useSheen } from '../lib/refraction';

type BtnVariant = 'primary' | 'accent' | 'glass' | 'tinted' | 'plain' | 'ghost' | 'danger' | 'danger-solid';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: BtnVariant;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  block?: boolean;
  icon?: ReactNode;
  to?: string;
  loading?: boolean;
}

const cls = (...xs: unknown[]) => xs.filter((x): x is string => typeof x === 'string' && x.length > 0).join(' ');
export { cls };

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'plain', size = 'md', block, icon, to, loading, className, children, ...rest },
  ref,
) {
  const c = cls('btn', `btn-${variant}`, size !== 'md' && `btn-${size}`, block && 'btn-block', variant === 'glass' && 'glass', className);
  const inner = (
    <>
      {loading ? <span className="spinner" style={{ width: 16, height: 16 }} /> : icon}
      {children}
    </>
  );
  if (to)
    return (
      <Link to={to} className={c} viewTransition>
        {inner}
      </Link>
    );
  return (
    <button ref={ref} className={c} {...rest} disabled={rest.disabled || loading}>
      {inner}
    </button>
  );
});

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  variant?: 'glass' | 'plain' | 'clear' | 'solid';
  active?: boolean;
  count?: number;
  dot?: boolean;
  to?: string;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, size = 'md', variant = 'glass', active, count, dot, to, className, children, ...rest },
  ref,
) {
  const c = cls('icon-btn', size !== 'md' && size, variant === 'glass' && 'glass', variant === 'clear' && 'glass glass-clear', variant === 'plain' && 'plain', variant === 'solid' && 'solid', active && 'active', className);
  const badge = count ? <span className="count">{count > 9 ? '9+' : count}</span> : dot ? <span className="dot" /> : null;
  if (to)
    return (
      <Link to={to} className={c} aria-label={label} title={label} viewTransition>
        {children}
        {badge}
      </Link>
    );
  return (
    <button ref={ref} className={c} aria-label={label} title={rest.title ?? label} {...rest}>
      {children}
      {badge}
    </button>
  );
});

export function Glass({ as: Tag = 'div', className, children, sheen, ...rest }: { as?: 'div' | 'section' | 'nav' | 'aside' | 'header' | 'footer'; className?: string; children?: ReactNode; sheen?: boolean } & React.HTMLAttributes<HTMLElement>) {
  const ref = useRef<HTMLElement>(null);
  useSheen(sheen ? ref : { current: null });
  return (
    <Tag ref={ref as never} className={cls('glass', className)} {...rest}>
      {children}
    </Tag>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  size,
  block,
  label,
}: {
  value: T;
  options: { value: T; label: ReactNode }[];
  onChange: (v: T) => void;
  size?: 'sm';
  block?: boolean;
  label?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [thumb, setThumb] = useState<{ x: number; w: number } | null>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      const btn = el.querySelector<HTMLButtonElement>('button[aria-pressed="true"]');
      if (btn) setThumb({ x: btn.offsetLeft, w: btn.offsetWidth });
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [value, options.length]);
  return (
    <div ref={ref} className={cls('segmented', size, block && 'block')} role="group" aria-label={label}>
      {thumb && <span className="seg-thumb" style={{ width: thumb.w, transform: `translateX(${thumb.x}px)` }} />}
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Switch({ checked, onChange, label, size, accent, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; size?: 'sm'; accent?: boolean; disabled?: boolean }) {
  return <button type="button" role="switch" aria-checked={checked} aria-label={label} disabled={disabled} className={cls('switch', size, accent && 'accent')} onClick={() => onChange(!checked)} />;
}

export function Badge({ children, tone }: { children: ReactNode; tone?: 'solid' | 'live' | 'new' | 'green' | 'amber' | 'red' }) {
  return <span className={cls('badge', tone)}>{children}</span>;
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd>{children}</kbd>;
}

export function Avatar({ name, color, size = 36, image, className }: { name: string; color?: string; size?: number; image?: string; className?: string }) {
  return (
    <span className={cls('avatar', className)} style={{ ['--s' as string]: `${size}px`, ['--c' as string]: color }} aria-hidden="true">
      {image ? <img src={image} alt="" /> : name.trim()[0]?.toUpperCase()}
    </span>
  );
}

export function Progress({ value, accent, className, style }: { value: number; accent?: boolean; className?: string; style?: React.CSSProperties }) {
  return (
    <div className={cls('progress', accent && 'accent', className)} style={style} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(value * 100)}>
      <i style={{ width: `${Math.max(2, Math.min(100, value * 100))}%` }} />
    </div>
  );
}

export function EmptyState({ icon, title, body, actions }: { icon: ReactNode; title: string; body?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="state">
      <div className="state-icon">{icon}</div>
      <h2>{title}</h2>
      {body && <p>{body}</p>}
      {actions && <div className="actions">{actions}</div>}
    </div>
  );
}

export function ErrorState({ error, retry, title = 'Couldn’t load this page' }: { error?: unknown; retry?: () => void; title?: string }) {
  const msg = error instanceof Error ? error.message : 'Something went wrong talking to your server.';
  const notFound = !!error && typeof error === 'object' && 'status' in error && (error as { status: number }).status === 404;
  if (notFound)
    return (
      <div className="state" role="alert">
        <div className="state-icon">
          <AlertTriangle />
        </div>
        <h2>Not on Harbor</h2>
        <p>{msg} It may have been removed or renamed.</p>
        <div className="actions">
          <Button variant="accent" to="/">
            Go Home
          </Button>
          <Button variant="plain" to="/search">
            Search
          </Button>
        </div>
      </div>
    );
  return (
    <div className="state error" role="alert">
      <div className="state-icon">
        <AlertTriangle />
      </div>
      <h2>{title}</h2>
      <p>{msg}</p>
      {retry && (
        <div className="actions">
          <Button variant="plain" icon={<RotateCw />} onClick={retry}>
            Try again
          </Button>
          <Button variant="ghost" to="/console/diagnostics">
            Server status
          </Button>
        </div>
      )}
    </div>
  );
}

export function Skel({ w, h, r, className, style }: { w?: number | string; h?: number | string; r?: number; className?: string; style?: React.CSSProperties }) {
  return <div className={cls('skel', className)} style={{ width: w, height: h, borderRadius: r, ...style }} aria-hidden="true" />;
}

export function Spinner({ size = 20 }: { size?: number }) {
  return <span className="spinner" style={{ width: size, height: size }} role="status" aria-label="Loading" />;
}
