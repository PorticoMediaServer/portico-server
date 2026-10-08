import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import { Bell, ChevronLeft } from 'lucide-react';
import { ProfileButton } from '../app/ProfileMenu';
import { useNotifications } from '../services/queries';
import { useUi } from '../state/ui';
import { IconButton, cls } from './ui';

/**
 * Desktop: a large page title with optional trailing controls.
 * Phones: an iOS large title that hands off to a compact glass bar as you scroll.
 */
export function PageHeader({
  title,
  subtitle,
  actions,
  mobileActions,
  back,
  children,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  mobileActions?: ReactNode;
  back?: boolean | string;
  children?: ReactNode;
}) {
  const sentinel = useRef<HTMLDivElement>(null);
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const notes = useNotifications();
  const unread = (notes.data ?? []).filter((n) => !n.read).length;
  const setNotifications = useUi((s) => s.setNotifications);

  useEffect(() => {
    const el = sentinel.current;
    if (!el) return;
    const io = new IntersectionObserver(([e]) => setCollapsed(!e.isIntersecting), { rootMargin: '-60px 0px 0px 0px' });
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const goBack = () => (typeof back === 'string' ? navigate(back, { viewTransition: true }) : navigate(-1));

  return (
    <>
      <div className="page-title-row desktop-only">
        <div>
          <h1 className="page-title">{title}</h1>
          {subtitle && <div className="page-sub">{subtitle}</div>}
        </div>
        {actions && <div className="section-tabs">{actions}</div>}
      </div>
      <div className="m-header">
        <div className="m-header-top">
          {back && (
            <button className="m-back" onClick={goBack} aria-label="Back">
              <ChevronLeft />
              Back
            </button>
          )}
          <span style={{ flex: 1 }} />
          {mobileActions}
          <IconButton label="Notifications" size="md" onClick={() => setNotifications(true)} count={unread}>
            <Bell />
          </IconButton>
          <ProfileButton size={36} />
        </div>
        <h1 className="m-large">{title}</h1>
        {subtitle && <div className="m-large-sub">{subtitle}</div>}
        <div ref={sentinel} style={{ height: 1 }} />
        {children}
      </div>
      <div className={cls('m-bar glass glass-chrome', collapsed && 'show')} aria-hidden={!collapsed}>
        {back && (
          <button className="m-back m-bar-start" onClick={goBack} aria-label="Back" tabIndex={collapsed ? 0 : -1}>
            <ChevronLeft />
          </button>
        )}
        <div className="m-bar-title ellipsis">{title}</div>
        <div className="m-bar-end">{mobileActions}</div>
      </div>
    </>
  );
}
