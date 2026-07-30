import { NavLink, useLocation } from 'react-router-dom';
import { cn } from '@/lib/cn';
import type { IconType } from '@/lib/icon';

type Props = {
  to: string;
  icon: IconType;
  label: string;
  /** When set, item is active only if these query keys are absent on the current URL. */
  absentQueryKeys?: string[];
};

function paramsEqualOnDefinedKeys(target: URLSearchParams, current: URLSearchParams) {
  for (const [k, v] of target.entries()) {
    if (current.get(k) !== v) return false;
  }
  return true;
}

/** Level-2 sidebar row for /devices filters (system_name sub-menu). */
export function DeviceSidebarNavItem({ to, icon: Icon, label, absentQueryKeys }: Props) {
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
        'ml-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors',
        'text-zinc-300 hover:bg-zinc-800/60 hover:text-zinc-100',
        isActive && 'bg-zinc-800 text-zinc-100',
      )}
      title={label}
    >
      <Icon size={14} className="shrink-0 text-zinc-400" />
      <span className="truncate">{label}</span>
    </NavLink>
  );
}
