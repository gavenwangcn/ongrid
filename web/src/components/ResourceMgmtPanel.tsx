import { useCallback, useEffect, useState } from 'react';
import { Loader2, Save } from 'lucide-react';
import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';
import { usePermissions } from '@/store/me';
import { getResourceMgmt, setResourceMgmt, type ResourceMgmtConfig } from '@/api/reports';

const DEFAULTS: ResourceMgmtConfig = {
  enabled: true,
  cpu_peak_max_pct: 20,
  mem_peak_max_pct: 30,
  min_cpu_count: 8,
  min_mem_gb: 16,
};

export function ResourceMgmtPanel() {
  const { tr } = useI18n();
  const { canMutate } = usePermissions();
  const [cfg, setCfg] = useState<ResourceMgmtConfig>(DEFAULTS);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    void (async () => {
      try {
        const res = await getResourceMgmt();
        setCfg({ ...DEFAULTS, ...res });
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const onSave = useCallback(async () => {
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      const res = await setResourceMgmt(cfg);
      setCfg({ ...DEFAULTS, ...res });
      setSaved(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : tr('保存失败', 'Save failed'));
    } finally {
      setSaving(false);
    }
  }, [cfg, tr]);

  const field = (
    key: keyof ResourceMgmtConfig,
    label: string,
    unit: string,
    step = 1,
  ) => (
    <label className="block min-w-0">
      <span className="text-[10px] text-zinc-500">{label}</span>
      <div className="mt-0.5 flex items-center gap-1">
        <input
          type="number"
          step={step}
          min={0}
          disabled={!canMutate || !cfg.enabled}
          value={cfg[key] as number}
          onChange={(e) => {
            setSaved(false);
            setCfg((c) => ({ ...c, [key]: Number(e.target.value) }));
          }}
          className="w-full rounded border border-zinc-800/60 bg-transparent px-1.5 py-1 text-xs tabular-nums text-zinc-100 focus:border-zinc-600 focus:outline-none disabled:opacity-50"
        />
        {unit && <span className="shrink-0 text-[10px] text-zinc-600">{unit}</span>}
      </div>
    </label>
  );

  return (
    <section className="mt-5 rounded-lg border border-zinc-800/60 bg-zinc-900/20 px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h2 className="text-xs font-medium text-zinc-300">{tr('资源管理（降配建议）', 'Resource management (downsize hints)')}</h2>
          <p className="mt-0.5 text-[10px] leading-snug text-zinc-600">
            {tr(
              'CPU/内存峰值低于阈值且规格超过最小核数/内存时，报告标记可降配设备。',
              'Flag right-sizing when CPU/memory peaks stay low while specs exceed minimums.',
            )}
          </p>
        </div>
        <label className="flex shrink-0 items-center gap-1.5 text-[11px] text-zinc-500">
          <input
            type="checkbox"
            checked={cfg.enabled}
            disabled={!canMutate}
            onChange={(e) => {
              setSaved(false);
              setCfg((c) => ({ ...c, enabled: e.target.checked }));
            }}
            className="rounded border-zinc-700 bg-transparent"
          />
          {tr('启用', 'Enable')}
        </label>
      </div>

      {loading ? (
        <div className="mt-2 flex items-center gap-1.5 text-[11px] text-zinc-500">
          <Loader2 size={12} className="animate-spin" /> {tr('加载…', 'Loading…')}
        </div>
      ) : (
        <div className="mt-2.5 flex flex-wrap items-end gap-x-3 gap-y-2">
          <div className="grid min-w-0 flex-1 grid-cols-2 gap-x-3 gap-y-2 sm:grid-cols-4">
            {field('cpu_peak_max_pct', tr('CPU 峰值上限', 'CPU peak max'), '%')}
            {field('mem_peak_max_pct', tr('内存 峰值上限', 'Memory peak max'), '%')}
            {field('min_cpu_count', tr('最小 CPU 核数', 'Min CPU cores'), tr('核', 'cores'), 1)}
            {field('min_mem_gb', tr('最小内存', 'Min memory'), 'GB', 0.5)}
          </div>
          {canMutate && (
            <button
              type="button"
              onClick={() => void onSave()}
              disabled={saving}
              className={cn(
                'inline-flex shrink-0 items-center gap-1 rounded-md border px-2 py-1 text-[11px]',
                saved
                  ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300'
                  : 'border-indigo-600 bg-indigo-600/20 text-indigo-200 hover:bg-indigo-600/30',
              )}
            >
              {saving ? <Loader2 size={11} className="animate-spin" /> : <Save size={11} />}
              {saved ? tr('已保存', 'Saved') : tr('保存', 'Save')}
            </button>
          )}
        </div>
      )}
      {error && <p className="mt-1.5 text-[11px] text-red-400">{error}</p>}
    </section>
  );
}
