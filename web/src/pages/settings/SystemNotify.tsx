import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Bell, Loader2, Save } from 'lucide-react';
import {
  getSystemNotifyConfig,
  setSystemNotifyConfig,
  type SystemNotifyBinding,
  type SystemNotifyChannel,
} from '@/api/alerts';
import {
  ENVIRONMENT_TAG_LABELS,
  ENVIRONMENT_TAG_LABELS_EN,
  ENVIRONMENT_TAGS,
  type EnvironmentTag,
} from '@/api/environment';
import { ApiError } from '@/api/client';
import { Button, Card } from '@/components/ui';
import { usePermissions } from '@/store/me';
import { useI18n } from '@/i18n/locale';

function channelLabel(ch: SystemNotifyChannel): string {
  const suffix = ch.enabled ? '' : ' (disabled)';
  return `${ch.name} · ${ch.type}${suffix}`;
}

function bindingKey(systemName: string, environmentTag?: string): string {
  return `${systemName}\0${environmentTag ?? ''}`;
}

function envLabel(tag: string, tr: (zh: string, en: string) => string): string {
  if (!tag) return tr('全部环境', 'All environments');
  if (ENVIRONMENT_TAGS.includes(tag as EnvironmentTag)) {
    const zh = ENVIRONMENT_TAG_LABELS[tag as EnvironmentTag];
    const en = ENVIRONMENT_TAG_LABELS_EN[tag as EnvironmentTag];
    return tr(zh, en);
  }
  return tag;
}

export default function SystemNotifySettingsPage() {
  const { tr } = useI18n();
  const { isAdmin } = usePermissions();
  const [channels, setChannels] = useState<SystemNotifyChannel[]>([]);
  const [bindings, setBindings] = useState<SystemNotifyBinding[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const load = useCallback(async () => {
    setErr(null);
    try {
      const res = await getSystemNotifyConfig();
      setChannels(res.channels ?? []);
      setBindings(res.bindings ?? []);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const rows = useMemo(() => {
    const seen = new Set<string>();
    const out: SystemNotifyBinding[] = [];
    for (const b of bindings) {
      const key = bindingKey(b.system_name, b.environment_tag);
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(b);
    }
    return out.sort((a, b) => {
      const s = a.system_name.localeCompare(b.system_name);
      if (s !== 0) return s;
      return (a.environment_tag ?? '').localeCompare(b.environment_tag ?? '');
    });
  }, [bindings]);

  function updateBinding(
    systemName: string,
    environmentTag: string | undefined,
    patch: Partial<SystemNotifyBinding>,
  ) {
    const key = bindingKey(systemName, environmentTag);
    setBindings((prev) =>
      prev.map((b) => (bindingKey(b.system_name, b.environment_tag) === key ? { ...b, ...patch } : b)),
    );
    setSaved(false);
  }

  function toggleChannel(systemName: string, environmentTag: string | undefined, channelId: number) {
    const key = bindingKey(systemName, environmentTag);
    setBindings((prev) =>
      prev.map((b) => {
        if (bindingKey(b.system_name, b.environment_tag) !== key) return b;
        const cur = b.channel_ids ?? [];
        const next = cur.includes(channelId) ? cur.filter((id) => id !== channelId) : [...cur, channelId];
        return { ...b, channel_ids: next };
      }),
    );
    setSaved(false);
  }

  async function onSave() {
    if (!isAdmin) return;
    setSaving(true);
    setErr(null);
    setSaved(false);
    try {
      const payload = bindings.filter((b) => (b.channel_ids?.length ?? 0) > 0);
      const res = await setSystemNotifyConfig(payload);
      setChannels(res.channels ?? []);
      setBindings(res.bindings ?? []);
      setSaved(true);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <Link
        to="/settings/notifications"
        className="inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-zinc-300"
      >
        <ArrowLeft size={13} /> {tr('返回通知渠道', 'Back to notification channels')}
      </Link>

      <div className="rounded-lg border border-zinc-800/60 bg-zinc-900/30 px-4 py-3 text-[12px] text-zinc-400">
        <div className="mb-1 flex items-center gap-2 text-zinc-200">
          <Bell size={14} className="text-indigo-400" />
          <span className="font-medium">{tr('系统告警路由', 'System alert routing')}</span>
        </div>
        {tr(
          '按业务系统与环境（设备上的 system_name / environment_tag）将告警投递到指定通知群。可选「全部环境」作为该系统下任意环境的默认渠道；也可为 dev / test / prod 分别指定。',
          'Route alerts by business system and environment (device system_name / environment_tag) to dedicated groups. Use “All environments” as the system-wide default, or pin dev / test / prod separately.',
        )}
      </div>

      {err && (
        <Card className="p-4 text-sm text-red-300">
          {tr('操作失败：', 'Failed: ')}{err}
        </Card>
      )}

      {loading ? (
        <div className="py-16 text-center text-sm text-zinc-500">{tr('加载中…', 'Loading…')}</div>
      ) : channels.length === 0 ? (
        <Card className="p-6 text-sm text-zinc-400">
          {tr('请先在', 'Create notification channels under ')}
          <Link to="/settings/notifications" className="text-indigo-300 hover:text-indigo-200">
            {tr('通知渠道', 'Notifications')}
          </Link>
          {tr(' 中配置至少一个渠道。', ' first.')}
        </Card>
      ) : rows.length === 0 ? (
        <Card className="p-6 text-sm text-zinc-400">
          {tr('暂无业务系统。请在设备 / Edge 详情中填写 system_name 后再配置。', 'No business systems yet. Set system_name on devices / edges first.')}
        </Card>
      ) : (
        <Card className="divide-y divide-zinc-800/60 p-0">
          {rows.map((row) => {
            const selected = new Set(row.channel_ids ?? []);
            const env = row.environment_tag ?? '';
            return (
              <div key={bindingKey(row.system_name, env)} className="px-4 py-3">
                <div className="mb-2 flex flex-wrap items-center gap-3">
                  <span className="text-sm font-medium text-zinc-100">{row.system_name}</span>
                  <select
                    value={env}
                    disabled={!isAdmin}
                    onChange={(e) =>
                      updateBinding(row.system_name, row.environment_tag, {
                        environment_tag: e.target.value,
                      })
                    }
                    className="rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1 text-xs text-zinc-300"
                  >
                    <option value="">{tr('全部环境', 'All environments')}</option>
                    {ENVIRONMENT_TAGS.map((tag) => (
                      <option key={tag} value={tag}>
                        {envLabel(tag, tr)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex flex-wrap gap-2">
                  {channels.map((ch) => {
                    const on = selected.has(ch.id);
                    return (
                      <button
                        key={ch.id}
                        type="button"
                        disabled={!isAdmin}
                        onClick={() => toggleChannel(row.system_name, row.environment_tag, ch.id)}
                        className={
                          on
                            ? 'rounded-md border border-indigo-600/50 bg-indigo-600/20 px-2.5 py-1 text-xs text-indigo-200'
                            : 'rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1 text-xs text-zinc-400 hover:bg-zinc-800'
                        }
                      >
                        {channelLabel(ch)}
                      </button>
                    );
                  })}
                </div>
                {selected.size === 0 && (
                  <p className="mt-2 text-xs text-zinc-500">
                    {tr('未选择 — 该范围告警走全局通知渠道', 'None selected — alerts use global channels')}
                  </p>
                )}
              </div>
            );
          })}
        </Card>
      )}

      {isAdmin && rows.length > 0 && channels.length > 0 && (
        <div className="flex items-center justify-end gap-3">
          {saved && <span className="text-xs text-emerald-400">{tr('已保存', 'Saved')}</span>}
          <Button type="button" disabled={saving} onClick={() => void onSave()} className="inline-flex items-center gap-1.5">
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            {tr('保存', 'Save')}
          </Button>
        </div>
      )}
    </div>
  );
}
