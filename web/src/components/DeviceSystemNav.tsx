import { useCallback, useEffect, useState } from 'react';
import { ChevronRight, LayoutGrid } from 'lucide-react';
import { NavLink, useLocation } from 'react-router-dom';
import { listDeviceSystemNames } from '@/api/devices';
import { onDevicesChanged } from '@/lib/events';
import { useI18n } from '@/i18n/locale';
import { cn } from '@/lib/cn';
import { DeviceSidebarNavItem } from '@/components/DeviceSidebarNavItem';

const STORAGE_KEY = 'sidebar.section.device-systems';

/** Dynamic 设备 sub-menu: 全部设备 (expandable) → per system_name. */
export function DeviceSystemNav() {
  const { tr } = useI18n();
  const location = useLocation();
  const [systems, setSystems] = useState<string[]>([]);
  const [open, setOpen] = useState(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw === 'open') return true;
      if (raw === 'closed') return false;
    } catch {
      /* ignore */
    }
    return true;
  });

  const load = useCallback(async () => {
    try {
      const resp = await listDeviceSystemNames();
      setSystems(resp.items ?? []);
    } catch {
      setSystems([]);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => onDevicesChanged(load), [load]);

  const currentParams = new URLSearchParams(location.search);
  const hasSystemFilter = Boolean(currentParams.get('system_name'));
  const isAllDevicesActive =
    location.pathname === '/devices' && !hasSystemFilter;

  useEffect(() => {
    if (!location.pathname.startsWith('/devices')) return;
    setOpen(true);
    try {
      localStorage.setItem(STORAGE_KEY, 'open');
    } catch {
      /* ignore */
    }
  }, [location.pathname, hasSystemFilter]);

  const toggle = () => {
    setOpen((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(STORAGE_KEY, next ? 'open' : 'closed');
      } catch {
        /* ignore */
      }
      return next;
    });
  };

  return (
    <div className="ml-2">
      <div
        className={cn(
          'flex items-center rounded-md transition-colors',
          isAllDevicesActive && 'bg-zinc-800',
        )}
      >
        <NavLink
          to="/devices"
          className={cn(
            'flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors',
            'text-zinc-300 hover:bg-zinc-800/60 hover:text-zinc-100',
            isAllDevicesActive && 'text-zinc-100',
          )}
          title={tr('全部设备', 'All devices')}
        >
          <LayoutGrid size={14} className="shrink-0 text-zinc-400" />
          <span className="truncate">{tr('全部设备', 'All devices')}</span>
        </NavLink>
        {systems.length > 0 ? (
          <button
            type="button"
            onClick={toggle}
            aria-expanded={open}
            aria-label={
              open
                ? tr('收起系统列表', 'Collapse system list')
                : tr('展开系统列表', 'Expand system list')
            }
            className="mr-0.5 rounded p-1 text-zinc-600 transition-colors hover:bg-zinc-800/60 hover:text-zinc-300"
          >
            <ChevronRight
              size={11}
              className={cn('transition-transform duration-150', open && 'rotate-90')}
            />
          </button>
        ) : null}
      </div>
      {open && systems.length > 0 ? (
        <div className="ml-2 mt-0.5 space-y-0.5">
          {systems.map((name) => (
            <DeviceSidebarNavItem
              key={name}
              to={`/devices?system_name=${encodeURIComponent(name)}`}
              label={name}
              nested
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
