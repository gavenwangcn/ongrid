import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Loader2, Save, Sparkles } from 'lucide-react';
import { ApiError } from '@/api/client';
import { getReportModel, setReportModel, type ReportModelConfig } from '@/api/reports';
import { Button, Card } from '@/components/ui';
import { ProviderIcon } from '@/components/icons/Provider';
import { usePermissions } from '@/store/me';
import { useI18n } from '@/i18n/locale';

export default function ReportModelSettingsPage() {
  const { tr } = useI18n();
  const { canMutate } = usePermissions();
  const [cfg, setCfg] = useState<ReportModelConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const [usePlatform, setUsePlatform] = useState(true);
  const [provider, setProvider] = useState('');
  const [model, setModel] = useState('');

  const load = useCallback(async () => {
    setErr(null);
    try {
      const res = await getReportModel();
      setCfg(res);
      setUsePlatform(res.use_platform_default);
      setProvider(res.provider || res.platform_default?.provider || '');
      setModel(res.model || res.platform_default?.model || '');
    } catch (e) {
      setErr((e as Error).message || tr('加载失败', 'Failed to load'));
    } finally {
      setLoading(false);
    }
  }, [tr]);

  useEffect(() => {
    void load();
  }, [load]);

  const selectedProvider = useMemo(
    () => cfg?.providers?.find((p) => p.id === provider),
    [cfg, provider],
  );

  const modelOptions = useMemo(() => {
    if (!selectedProvider) return [];
    const list = [...(selectedProvider.models ?? [])];
    if (selectedProvider.model && !list.includes(selectedProvider.model)) {
      list.unshift(selectedProvider.model);
    }
    return list;
  }, [selectedProvider]);

  useEffect(() => {
    if (!selectedProvider || modelOptions.length === 0) return;
    if (!model || !modelOptions.includes(model)) {
      setModel(modelOptions[0]);
    }
  }, [selectedProvider, modelOptions, model]);

  async function onSave() {
    if (!canMutate) return;
    setSaving(true);
    setErr(null);
    setSaved(false);
    try {
      const body = usePlatform ? { provider: '', model: '' } : { provider, model };
      const res = await setReportModel(body);
      setCfg(res);
      setUsePlatform(res.use_platform_default);
      setProvider(res.provider || res.platform_default?.provider || '');
      setModel(res.model || res.platform_default?.model || '');
      setSaved(true);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  const platformLabel = cfg
    ? `${cfg.platform_default.provider}${cfg.platform_default.model ? ` / ${cfg.platform_default.model}` : ''}`
    : '';

  return (
    <main className="anim-fade flex flex-1 flex-col overflow-hidden">
      <header className="app-header border-b border-zinc-800/60 px-6 py-4">
        <Link to="/reports" className="mb-2 inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-zinc-300">
          <ArrowLeft size={13} /> {tr('返回报告', 'Back to reports')}
        </Link>
        <div>
          <h1 className="text-base font-semibold text-zinc-100">{tr('报告模型', 'Report model')}</h1>
          <p className="mt-0.5 text-xs text-zinc-500">
            {tr(
              '为日报/周报/月报的 AI 评估指定固定模型；未指定时使用平台默认 LLM。',
              'Pin a fixed LLM for daily/weekly/monthly report AI evaluation; otherwise the platform default is used.',
            )}
          </p>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-6 py-5">
        {err && (
          <div className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
            <span>{err}</span>
            <Link
              to="/settings/llm"
              className="shrink-0 font-medium text-amber-700 hover:text-amber-900 dark:text-amber-200 dark:hover:text-amber-100"
            >
              {tr('去配置 LLM', 'Configure LLM')}
            </Link>
          </div>
        )}

        {loading ? (
          <div className="py-16 text-center text-sm text-zinc-500">{tr('加载中…', 'Loading…')}</div>
        ) : (
          <Card className="mx-auto max-w-xl p-0">
            <div className="border-b border-zinc-800/60 px-4 py-3">
              <div className="flex items-center gap-2 text-sm text-zinc-200">
                <Sparkles size={14} className="text-indigo-400" />
                {tr('Reporter 评估模型', 'Reporter evaluation model')}
              </div>
              <p className="mt-1 text-xs text-zinc-500">
                {tr(
                  '仅影响报告生成 worker，不影响聊天或 RCA 调查。',
                  'Affects the report generation worker only — not chat or RCA investigations.',
                )}
              </p>
            </div>

            <div className="divide-y divide-zinc-800/60">
              <label className="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-zinc-900/30">
                <input
                  type="radio"
                  name="report-model-mode"
                  className="mt-0.5"
                  checked={usePlatform}
                  disabled={!canMutate}
                  onChange={() => setUsePlatform(true)}
                />
                <span>
                  <span className="block text-sm text-zinc-200">{tr('跟随平台默认', 'Use platform default')}</span>
                  <span className="mt-0.5 block text-xs text-zinc-500">
                    {platformLabel
                      ? tr(`当前默认：${platformLabel}`, `Current default: ${platformLabel}`)
                      : tr('与首页 / 设置中的默认 LLM 一致', 'Same as the Home / Settings default LLM')}
                  </span>
                </span>
              </label>

              <label className="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-zinc-900/30">
                <input
                  type="radio"
                  name="report-model-mode"
                  className="mt-0.5"
                  checked={!usePlatform}
                  disabled={!canMutate || (cfg?.providers?.length ?? 0) === 0}
                  onChange={() => setUsePlatform(false)}
                />
                <span className="flex-1">
                  <span className="block text-sm text-zinc-200">{tr('固定模型', 'Fixed model')}</span>
                  {(cfg?.providers?.length ?? 0) === 0 ? (
                    <span className="mt-0.5 block text-xs text-zinc-500">
                      {tr('请先在 LLM 设置中配置至少一个 provider', 'Configure at least one LLM provider first')}
                    </span>
                  ) : (
                    <div className="mt-3 grid gap-3 sm:grid-cols-2">
                      <div>
                        <label className="mb-1 block text-xs text-zinc-500">{tr('Provider', 'Provider')}</label>
                        <select
                          value={provider}
                          disabled={!canMutate || usePlatform}
                          onChange={(e) => setProvider(e.target.value)}
                          className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-sm text-zinc-200"
                        >
                          {(cfg?.providers ?? []).map((p) => (
                            <option key={p.id} value={p.id}>
                              {p.label || p.id}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div>
                        <label className="mb-1 block text-xs text-zinc-500">{tr('模型', 'Model')}</label>
                        <select
                          value={model}
                          disabled={!canMutate || usePlatform || modelOptions.length === 0}
                          onChange={(e) => setModel(e.target.value)}
                          className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-sm text-zinc-200"
                        >
                          {modelOptions.map((m) => (
                            <option key={m} value={m}>
                              {m}
                            </option>
                          ))}
                        </select>
                      </div>
                    </div>
                  )}
                  {!usePlatform && selectedProvider && (
                    <span className="mt-2 inline-flex items-center gap-1.5 text-xs text-zinc-500">
                      <ProviderIcon provider={selectedProvider.id} size={12} />
                      {tr('报告生成将始终使用此模型', 'Report generation will always use this model')}
                    </span>
                  )}
                </span>
              </label>
            </div>

            {canMutate && (
              <div className="flex items-center justify-end gap-3 border-t border-zinc-800/60 px-4 py-3">
                {saved && <span className="text-xs text-emerald-400">{tr('已保存', 'Saved')}</span>}
                <Button
                  type="button"
                  onClick={() => void onSave()}
                  disabled={saving || (!usePlatform && !provider)}
                  className="inline-flex items-center gap-1.5"
                >
                  {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
                  {tr('保存', 'Save')}
                </Button>
              </div>
            )}
          </Card>
        )}
      </div>
    </main>
  );
}
