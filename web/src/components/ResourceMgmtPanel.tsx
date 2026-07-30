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
    <label className="block">
      <span className="text-[11px] text-zinc-500">{label}</span>
      <div className="mt-1 flex items-center gap-1.5">
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
          className="w-full rounded-md border border-zinc-800 bg-zinc-950/60 px-2 py-1.5 text-sm tabular-nums text-zinc-100 disabled:opacity-50"
        />
        {unit && <span className="shrink-0 text-[11px] text-zinc-600">{unit}</span>}
      </div>
    </label>
  );

  return (
    <section className="border-b border-zinc-800/60 bg-zinc-950/30 px-6 py-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-medium text-zinc-200">{tr('资源管理（降配建议）', 'Resource management (downsize hints)')}</h2>
          <p className="mt-0.5 max-w-3xl text-xs text-zinc-500">
            {tr(
              '报告集群态势会按以下规则标记可降配设备：周期 CPU 峰值、内存峰值均低于阈值，且规格超过最小核数/内存。飞书通知仍只发汇总。',
              'Cluster reports flag right-sizing candidates when period CPU and memory peaks stay below thresholds while specs exceed minimums. Feishu still receives summary only.',
            )}
          </p>
        </div>
        <label className="flex items-center gap-2 text-xs text-zinc-400">
          <input
            type="checkbox"
            checked={cfg.enabled}
            disabled={!canMutate}
            onChange={(e) => {
              setSaved(false);
              setCfg((c) => ({ ...c, enabled: e.target.checked }));
            }}
            className="rounded border-zinc-700"
          />
          {tr('启用降配建议', 'Enable downsize hints')}
        </label>
      </div>

      {loading ? (
        <div className="mt-3 flex items-center gap-2 text-xs text-zinc-500">
          <Loader2 size={14} className="animate-spin" /> {tr('加载配置…', 'Loading…')}
        </div>
      ) : (
        <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-5">
          {field('cpu_peak_max_pct', tr('CPU 峰值上限', 'CPU peak max'), '%')}
          {field('mem_peak_max_pct', tr('内存 峰值上限', 'Memory peak max'), '%')}
          {field('min_cpu_count', tr('最小 CPU 核数', 'Min CPU cores'), tr('核', 'cores'), 1)}
          {field('min_mem_gb', tr('最小内存', 'Min memory'), 'GB', 0.5)}
          {canMutate && (
            <div className="flex items-end">
              <button
                type="button"
                onClick={() => void onSave()}
                disabled={saving}
                className={cn(
                  'inline-flex w-full items-center justify-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs',
                  saved
                    ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300'
                    : 'border-indigo-600 bg-indigo-600/20 text-indigo-200 hover:bg-indigo-600/30',
                )}
              >
                {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
                {saved ? tr('已保存', 'Saved') : tr('保存', 'Save')}
              </button>
            </div>
          )}
        </div>
      )}
      {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
    </section>
  );
}
