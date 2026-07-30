import { NavLink, useLocation } from 'react-router-dom';
import { cn } from '@/lib/cn';
import type { IconType } from '@/lib/icon';

type Props = {
  to: string;
  icon?: IconType;
  label: string;
  /** Nested under 全部设备 — extra indent, no icon. */
  nested?: boolean;
  /** When set, item is active only if these query keys are absent on the current URL. */
  absentQueryKeys?: string[];
};

function paramsEqualOnDefinedKeys(target: URLSearchParams, current: URLSearchParams) {
  for (const [k, v] of target.entries()) {
    if (current.get(k) !== v) return false;
  }
  return true;
}

/** Sidebar row for /devices filters (system_name sub-menu under 全部设备). */
export function DeviceSidebarNavItem({ to, icon: Icon, label, nested, absentQueryKeys }: Props) {
  const location = useLocation();
  const [targetPath, targetQuery] = to.split('?');
  const hasQuery = Boolean(targetQuery);
  const targetParams = hasQuery ? new URLSearchParams(targetQuery) : null;
  const currentParams = new URLSearchParams(location.search);

  let isActive = false;
  if (location.pathname === targetPath) {
    if (hasQuery) {
      isActive = paramsEqualOnDefinedKeys(targetParams!, currentParams);
    } else if (absentQueryKeys?.length) {
      isActive = absentQueryKeys.every((k) => !currentParams.get(k));
    } else {
      isActive = true;
    }
  }

  return (
    <NavLink
      to={to}
      className={cn(
        nested ? 'ml-0' : 'ml-2',
        'flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors',
        nested ? 'text-zinc-400 hover:text-zinc-200' : 'text-zinc-300 hover:text-zinc-100',
        'hover:bg-zinc-800/60',
        isActive && 'bg-zinc-800 text-zinc-100',
      )}
      title={label}
    >
      {Icon ? <Icon size={14} className="shrink-0 text-zinc-400" /> : null}
      <span className="truncate">{label}</span>
    </NavLink>
  );
}
