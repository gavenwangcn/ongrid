import { useCallback, useEffect, useState } from 'react';
import { Building2, LayoutGrid } from 'lucide-react';
import { listDeviceSystemNames } from '@/api/devices';
import { onDevicesChanged } from '@/lib/events';
import { useI18n } from '@/i18n/locale';
import { DeviceSidebarNavItem } from '@/components/DeviceSidebarNavItem';

/** Dynamic 设备 sub-menu: 全部 + one entry per distinct system_name. */
export function DeviceSystemNav() {
  const { tr } = useI18n();
  const [systems, setSystems] = useState<string[]>([]);

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

  return (
    <>
      <DeviceSidebarNavItem
        to="/devices"
        icon={LayoutGrid}
        label={tr('全部设备', 'All devices')}
        absentQueryKeys={['system_name']}
      />
      {systems.map((name) => (
        <DeviceSidebarNavItem
          key={name}
          to={`/devices?system_name=${encodeURIComponent(name)}`}
          icon={Building2}
          label={name}
        />
      ))}
    </>
  );
}
