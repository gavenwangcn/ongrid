import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ChevronLeft, ChevronRight, Plus, Settings, Sparkles } from 'lucide-react';
import { Modal } from '@/components/Modal';
import { cn } from '@/lib/cn';
import { relativeTime } from '@/lib/format';
import { usePoll } from '@/lib/usePoll';
import { usePermissions } from '@/store/me';
import { useI18n } from '@/i18n/locale';
import { ApiError } from '@/api/client';
import { listDevices } from '@/api/devices';
import {
  ENVIRONMENT_TAGS,
  ENVIRONMENT_TAG_LABELS,
  ENVIRONMENT_TAG_LABELS_EN,
  type EnvironmentTag,
} from '@/api/environment';
import {
  formatReportScope,
  generateNow,
  listReports,
  uniqueSystemNames,
  type ReportKind,
  type ReportListItem,
  type ReportStatus,
} from '@/api/reports';

const POLL_MS = 20_000;
const PAGE_SIZE = 20;

const STATUS_STYLE: Record<ReportStatus, string> = {
  ready: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
  generating: 'bg-indigo-500/15 text-indigo-300 border-indigo-500/30',
  pending: 'bg-zinc-700/40 text-zinc-300 border-zinc-600/40',
  failed: 'bg-red-500/15 text-red-300 border-red-500/30',
};

const STATUS_FILTERS: { key: string; zh: string; en: string }[] = [
  { key: '', zh: '全部', en: 'All' },
  { key: 'ready', zh: '已就绪', en: 'Ready' },
  { key: 'generating', zh: '生成中', en: 'Generating' },
  { key: 'failed', zh: '失败', en: 'Failed' },
];

const KIND_FILTERS: { key: string; zh: string; en: string }[] = [
  { key: '', zh: '全部', en: 'All' },
  { key: 'daily', zh: '日报', en: 'Daily' },
  { key: 'weekly', zh: '周报', en: 'Weekly' },
  { key: 'monthly', zh: '月报', en: 'Monthly' },
  { key: 'yearly', zh: '年报', en: 'Yearly' },
];

const GENERATE_KINDS: { key: ReportKind; zh: string; en: string }[] = [
  { key: 'daily', zh: '日报', en: 'Daily' },
  { key: 'weekly', zh: '周报', en: 'Weekly' },
  { key: 'monthly', zh: '月报', en: 'Monthly' },
  { key: 'yearly', zh: '年报', en: 'Yearly' },
];

const GENERATE_KIND_HINT: Record<ReportKind, { zh: string; en: string }> = {
  daily: {
    zh: '统计上一自然日（昨日 00:00–24:00）。',
    en: 'Covers the previous calendar day (yesterday 00:00–24:00).',
  },
  weekly: {
    zh: '统计上一自然周（周一至周日）。',
    en: 'Covers the previous ISO week (Monday through Sunday).',
  },
  monthly: {
    zh: '统计上一自然月。',
    en: 'Covers the previous calendar month.',
  },
  yearly: {
    zh: '统计上一自然年。',
    en: 'Covers the previous calendar year.',
  },
  custom: {
    zh: '按自定义周期统计。',
    en: 'Covers a custom period.',
  },
};

const GENERATE_KIND_ACTION: Record<ReportKind, { zh: string; en: string }> = {
  daily: { zh: '生成日报', en: 'Generate daily' },
  weekly: { zh: '生成周报', en: 'Generate weekly' },
  monthly: { zh: '生成月报', en: 'Generate monthly' },
  yearly: { zh: '生成年报', en: 'Generate yearly' },
  custom: { zh: '生成报告', en: 'Generate report' },
};

const KIND_ZH: Record<string, string> = { daily: '日报', weekly: '周报', monthly: '月报', yearly: '年报', custom: '自定义' };
const KIND_EN: Record<string, string> = { daily: 'Daily', weekly: 'Weekly', monthly: 'Monthly', yearly: 'Yearly', custom: 'Custom' };

const STATUS_ZH: Record<ReportStatus, string> = { ready: '已就绪', generating: '生成中', pending: '待生成', failed: '失败' };
const STATUS_EN: Record<ReportStatus, string> = { ready: 'Ready', generating: 'Generating', pending: 'Pending', failed: 'Failed' };

// periodLabel strips the localized kind prefix ("日报 · " / "Daily · ")
// from a stored title, leaving the locale-neutral date/period for the
// table's primary line. Falls back to the full title if there's no " · ".
function periodLabel(title: string): string {
  const i = title.indexOf(' · ');
  return i >= 0 ? title.slice(i + 3) : title;
}

export default function ReportsPage() {
  const { tr } = useI18n();
  const { canMutate } = usePermissions();
  const navigate = useNavigate();
  const [items, setItems] = useState<ReportListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [generating, setGenerating] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');
  const [kindFilter, setKindFilter] = useState('');
  const [page, setPage] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const [generateOpen, setGenerateOpen] = useState(false);
  const [systemNames, setSystemNames] = useState<string[]>([]);

  const load = useCallback(async () => {
    try {
      const res = await listReports({
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        status: statusFilter || undefined,
        kind: kindFilter || undefined,
      });
      setItems(res.reports ?? []);
    } finally {
      setLoading(false);
    }
  }, [statusFilter, kindFilter, page]);

  // Reset to the first page whenever a filter changes.
  useEffect(() => {
    setPage(0);
  }, [statusFilter, kindFilter]);

  useEffect(() => {
    void load();
  }, [load]);
  usePoll(load, POLL_MS);

  useEffect(() => {
    if (!generateOpen) return;
    let cancelled = false;
    listDevices()
      .then((r) => {
        if (!cancelled) setSystemNames(uniqueSystemNames(r.items ?? []));
      })
      .catch(() => {
        if (!cancelled) setSystemNames([]);
      });
    return () => {
      cancelled = true;
    };
  }, [generateOpen]);

  const onGenerate = useCallback(
    async (kind: ReportKind, systemName: string, environmentTag: EnvironmentTag | '') => {
      setGenerating(true);
      setErr(null);
      try {
        const rpt = await generateNow({
          kind,
          scope_json: formatReportScope({
            system_name: systemName,
            environment_tag: environmentTag,
          }),
        });
        setGenerateOpen(false);
        await load();
        navigate(`/reports/${rpt.id}`);
      } catch (e) {
        setErr(reportActionError(e, tr));
      } finally {
        setGenerating(false);
      }
    },
    [load, navigate, tr],
  );

  return (
    <main className="anim-fade flex flex-1 flex-col overflow-hidden">
      <header className="app-header border-b border-zinc-800/60 px-6 py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="text-base font-semibold text-zinc-100">{tr('报告', 'Reports')}</h1>
            <p className="mt-0.5 text-xs text-zinc-500">
              {tr('定时或手动生成的运维报告', 'Scheduled and on-demand ops reports')}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Link
              to="/reports/model"
              className="inline-flex items-center gap-1.5 rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
            >
              <Sparkles size={12} /> {tr('报告模型', 'Report model')}
            </Link>
            <Link
              to="/reports/schedules"
              className="inline-flex items-center gap-1.5 rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
            >
              <Settings size={12} /> {tr('定时生成', 'Scheduled')}
            </Link>
            {canMutate && (
              <button
                type="button"
                onClick={() => setGenerateOpen(true)}
                disabled={generating}
                className="inline-flex items-center gap-1.5 rounded-md border border-indigo-600 bg-indigo-600/20 px-2.5 py-1.5 text-xs text-indigo-200 hover:bg-indigo-600/30 disabled:opacity-50"
              >
                <Plus size={12} /> {tr('立即生成', 'Generate now')}
              </button>
            )}
          </div>
        </div>
      </header>

      <div className="flex flex-wrap items-center gap-4 border-b border-zinc-800 px-6 py-3 text-xs text-zinc-400">
        <FilterGroup label={tr('状态', 'Status')} options={STATUS_FILTERS} value={statusFilter} onChange={setStatusFilter} tr={tr} />
        <FilterGroup label={tr('类型', 'Kind')} options={KIND_FILTERS} value={kindFilter} onChange={setKindFilter} tr={tr} />
      </div>

      <div className="flex flex-1 flex-col overflow-y-auto px-6 py-5">
        {err && (
          <div className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
            <span>{err}</span>
            <Link to="/settings/llm" className="shrink-0 font-medium text-amber-700 hover:text-amber-900 dark:text-amber-200 dark:hover:text-amber-100">
              {tr('去配置', 'Configure')}
            </Link>
          </div>
        )}
        <div className="overflow-hidden rounded-xl border border-zinc-800/60 bg-zinc-900/40">
          <table className="w-full table-fixed text-sm">
            <colgroup>
              <col className="w-44" />
              <col />
              <col className="w-28" />
              <col className="w-24" />
              <col className="w-28" />
            </colgroup>
            <thead className="border-b border-zinc-800/60 bg-zinc-950/40 text-[11px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-5 py-3 text-left">{tr('周期', 'Period')}</th>
                <th className="px-4 py-3 text-left">{tr('报告', 'Report')}</th>
                <th className="px-4 py-3 text-left">{tr('类型', 'Kind')}</th>
                <th className="px-4 py-3 text-left">{tr('状态', 'Status')}</th>
                <th className="px-5 py-3 text-right">{tr('生成时间', 'Generated')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800/40">
              {loading && items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-zinc-500">
                    {tr('加载中…', 'Loading…')}
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-zinc-500">
                    {page > 0
                      ? tr('这一页没有报告', 'No reports on this page')
                      : tr('暂无报告。点右上角「立即生成」，或设一个定时任务。', 'No reports yet. Click "Generate now" or set up a schedule.')}
                  </td>
                </tr>
              ) : (
                items.map((r) => (
                  <tr
                    key={r.id}
                    className="cursor-pointer transition-colors hover:bg-zinc-900/40"
                    onClick={() => navigate(`/reports/${r.id}`)}
                  >
                    {/* Period leads the row (chronological scan), muted grey
                        like other tables' metadata columns. */}
                    <td className="truncate px-5 py-3 text-xs text-zinc-400">{periodLabel(r.title)}</td>
                    {/* REPORT cell — the summary is the personalized name;
                        text-xs + body-grey to match the skills table's
                        description column, not bright/large. Falls back to a
                        placeholder before content lands. */}
                    <td className="truncate px-4 py-3 text-xs text-zinc-300">
                      {r.summary
                        ? r.summary
                        : r.status === 'failed'
                          ? <span className="text-zinc-500">{tr('生成失败', 'Generation failed')}</span>
                          : <span className="text-zinc-500">{tr('生成中…', 'Generating…')}</span>}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3">
                      <span className="inline-flex items-center rounded-md border border-zinc-700 bg-zinc-800/50 px-2 py-0.5 text-xs text-zinc-300">
                        {tr(KIND_ZH[r.kind] ?? r.kind, KIND_EN[r.kind] ?? r.kind)}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-4 py-3">
                      <span className={cn('inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium', STATUS_STYLE[r.status])}>
                        {tr(STATUS_ZH[r.status], STATUS_EN[r.status])}
                      </span>
                    </td>
                    <td className="whitespace-nowrap px-5 py-3 text-right text-xs text-zinc-500">
                      {r.generated_at ? relativeTime(r.generated_at) : relativeTime(r.created_at)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination — no total count from the API, so next is enabled
            while a full page came back. */}
        {(page > 0 || items.length === PAGE_SIZE) && (
          <div className="flex items-center justify-end gap-2 py-3 text-xs text-zinc-400">
            <span className="mr-2 text-zinc-600">{tr(`第 ${page + 1} 页`, `Page ${page + 1}`)}</span>
            <button
              type="button"
              disabled={page === 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              className="inline-flex items-center gap-1 rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1 hover:bg-zinc-800 disabled:opacity-40"
            >
              <ChevronLeft size={13} /> {tr('上一页', 'Prev')}
            </button>
            <button
              type="button"
              disabled={items.length < PAGE_SIZE}
              onClick={() => setPage((p) => p + 1)}
              className="inline-flex items-center gap-1 rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-1 hover:bg-zinc-800 disabled:opacity-40"
            >
              {tr('下一页', 'Next')} <ChevronRight size={13} />
            </button>
          </div>
        )}
      </div>

      {generateOpen && (
        <GenerateReportModal
          systemNames={systemNames}
          generating={generating}
          onClose={() => setGenerateOpen(false)}
          onGenerate={(kind, systemName, environmentTag) => void onGenerate(kind, systemName, environmentTag)}
          tr={tr}
        />
      )}
    </main>
  );
}

function GenerateReportModal({
  systemNames,
  generating,
  onClose,
  onGenerate,
  tr,
}: {
  systemNames: string[];
  generating: boolean;
  onClose(): void;
  onGenerate(kind: ReportKind, systemName: string, environmentTag: EnvironmentTag | ''): void;
  tr: (zh: string, en: string) => string;
}) {
  const [kind, setKind] = useState<ReportKind>('weekly');
  const [systemName, setSystemName] = useState('');
  const [environmentTag, setEnvironmentTag] = useState<EnvironmentTag | ''>('');
  const hint = GENERATE_KIND_HINT[kind];
  const action = GENERATE_KIND_ACTION[kind];

  return (
    <Modal
      open
      onClose={onClose}
      size="sm"
      title={tr('立即生成报告', 'Generate report now')}
      footer={
        <>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-zinc-700 px-3 py-1.5 text-xs text-zinc-200 hover:bg-zinc-800"
          >
            {tr('取消', 'Cancel')}
          </button>
          <button
            type="button"
            onClick={() => onGenerate(kind, systemName, environmentTag)}
            disabled={generating}
            className="rounded-md border border-indigo-600 bg-indigo-600/20 px-3 py-1.5 text-xs text-indigo-200 hover:bg-indigo-600/30 disabled:opacity-50"
          >
            {generating ? tr('生成中…', 'Generating…') : tr(action.zh, action.en)}
          </button>
        </>
      }
    >
      <div className="space-y-3">
        <div>
          <span className="text-xs text-zinc-400">{tr('报告类型', 'Report type')}</span>
          <div className="mt-1.5 flex gap-1.5">
            {GENERATE_KINDS.map((k) => (
              <button
                key={k.key}
                type="button"
                onClick={() => setKind(k.key)}
                className={cn(
                  'rounded-md border px-2.5 py-1 text-xs',
                  kind === k.key
                    ? 'border-indigo-500 bg-indigo-500/15 text-indigo-200'
                    : 'border-zinc-700 text-zinc-300 hover:border-zinc-500',
                )}
              >
                {tr(k.zh, k.en)}
              </button>
            ))}
          </div>
        </div>
        <p className="text-xs text-zinc-500">
          {tr(hint.zh, hint.en)}
          {tr(' 可选择系统与环境标签，仅统计匹配的设备。', ' Optionally narrow by system and environment tag.')}
        </p>
        <label className="block text-xs text-zinc-400">
          {tr('系统范围', 'System scope')}
          <select
            value={systemName}
            onChange={(e) => setSystemName(e.target.value)}
            className="mt-1.5 w-full rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-2 text-sm text-zinc-100"
          >
            <option value="">{tr('全部系统', 'All systems')}</option>
            {systemNames.map((name) => (
              <option key={name} value={name}>{name}</option>
            ))}
          </select>
        </label>
        <label className="block text-xs text-zinc-400">
          {tr('环境标签', 'Environment tag')}
          <select
            value={environmentTag}
            onChange={(e) => setEnvironmentTag(e.target.value as EnvironmentTag | '')}
            className="mt-1.5 w-full rounded-md border border-zinc-700 bg-zinc-900 px-2.5 py-2 text-sm text-zinc-100"
          >
            <option value="">{tr('全部环境', 'All environments')}</option>
            {ENVIRONMENT_TAGS.map((tag) => (
              <option key={tag} value={tag}>
                {tr(ENVIRONMENT_TAG_LABELS[tag], ENVIRONMENT_TAG_LABELS_EN[tag])}
              </option>
            ))}
          </select>
        </label>
        {systemNames.length === 0 && (
          <p className="text-[11px] text-zinc-600">
            {tr('未找到已填系统名称的设备；将统计全部设备。可在设备元数据中填写系统名称。', 'No devices with a system name yet — report will cover all devices. Set system name on device metadata.')}
          </p>
        )}
      </div>
    </Modal>
  );
}

function reportActionError(e: unknown, tr: (zh: string, en: string) => string): string {
  if (e instanceof ApiError && e.code === 'not-wired-yet') {
    return tr('当前未配置 LLM provider，请先配置模型后再生成报告。', 'No LLM provider is configured. Configure a model before generating reports.');
  }
  if (e instanceof ApiError) return e.message;
  return (e as Error)?.message || tr('生成失败', 'Generation failed');
}

function FilterGroup({
  label,
  options,
  value,
  onChange,
  tr,
}: {
  label: string;
  options: { key: string; zh: string; en: string }[];
  value: string;
  onChange(v: string): void;
  tr: (zh: string, en: string) => string;
}) {
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-zinc-500">{label}</span>
      <div className="flex gap-1">
        {options.map((o) => (
          <button
            key={o.key}
            type="button"
            onClick={() => onChange(o.key)}
            className={cn(
              'rounded px-2 py-0.5 text-[11px]',
              value === o.key
                ? 'bg-indigo-500/15 text-indigo-200'
                : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200',
            )}
          >
            {tr(o.zh, o.en)}
          </button>
        ))}
      </div>
    </div>
  );
}
