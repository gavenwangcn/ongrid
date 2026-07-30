import { request } from './client';

// Reports API (HLD-014). Scheduled operational reports — list + detail
// + manual generate + schedule CRUD. Backend routes under /v1/reports
// and /v1/report-schedules.

import type { EnvironmentTag } from './environment';

export type ReportStatus = 'pending' | 'generating' | 'ready' | 'failed';
export type ReportKind = 'daily' | 'weekly' | 'monthly' | 'custom';

export type ReportSection = 'cluster' | 'logs' | 'alerts';

export const REPORT_SECTION_DEFS: {
  key: ReportSection;
  zh: string;
  en: string;
  descZh: string;
  descEn: string;
}[] = [
  {
    key: 'cluster',
    zh: '集群态势',
    en: 'Cluster posture',
    descZh: '资源水位与监控覆盖',
    descEn: 'Resource & coverage',
  },
  {
    key: 'logs',
    zh: '应用日志',
    en: 'Application logs',
    descZh: '潜在错误',
    descEn: 'Potential errors',
  },
  {
    key: 'alerts',
    zh: '告警与处理',
    en: 'Alerts & response',
    descZh: '事件、处置与 agent 动作',
    descEn: 'Incidents & agent actions',
  },
];

export const ALL_REPORT_SECTIONS: ReportSection[] = REPORT_SECTION_DEFS.map((d) => d.key);

export const DEFAULT_DAILY_SECTIONS: ReportSection[] = ['cluster'];

/** Parsed fire time for preset report schedules (daily / weekly / monthly). */
export type ScheduleTimeConfig = {
  hour: number;
  minute: number;
  /** Cron weekday 0–6 (0 = Sunday). Used when kind = weekly. */
  weekday: number;
  /** Day of month 1–28. Used when kind = monthly. */
  dayOfMonth: number;
};

export const DEFAULT_SCHEDULE_TIME: ScheduleTimeConfig = {
  hour: 9,
  minute: 0,
  weekday: 1,
  dayOfMonth: 1,
};

export function defaultScheduleTime(_kind: ReportKind): ScheduleTimeConfig {
  return { ...DEFAULT_SCHEDULE_TIME };
}

/** Parse a 5-field cron into UI fields; falls back to 09:00 defaults. */
export function parseScheduleTime(kind: ReportKind, cronSpec?: string): ScheduleTimeConfig {
  const cfg = defaultScheduleTime(kind);
  if (!cronSpec?.trim()) return cfg;
  const parts = cronSpec.trim().split(/\s+/);
  if (parts.length < 5) return cfg;
  const minute = Number(parts[0]);
  const hour = Number(parts[1]);
  if (Number.isFinite(minute) && minute >= 0 && minute <= 59) cfg.minute = minute;
  if (Number.isFinite(hour) && hour >= 0 && hour <= 23) cfg.hour = hour;
  if (kind === 'weekly') {
    const wd = Number(parts[4]);
    if (Number.isFinite(wd) && wd >= 0 && wd <= 6) cfg.weekday = wd;
  }
  if (kind === 'monthly') {
    const dom = Number(parts[2]);
    if (Number.isFinite(dom) && dom >= 1 && dom <= 28) cfg.dayOfMonth = dom;
  }
  return cfg;
}

/** Build cron from preset kind + UI time fields. */
export function buildScheduleCron(kind: ReportKind, cfg: ScheduleTimeConfig): string {
  const minute = Math.min(59, Math.max(0, cfg.minute));
  const hour = Math.min(23, Math.max(0, cfg.hour));
  switch (kind) {
    case 'daily':
      return `${minute} ${hour} * * *`;
    case 'weekly':
      return `${minute} ${hour} * * ${cfg.weekday}`;
    case 'monthly':
      return `${minute} ${hour} ${cfg.dayOfMonth} * *`;
    default:
      return '';
  }
}

export type ReportScope = {
  /** @deprecated use system_names */
  system_name?: string;
  system_names?: string[];
  environment_tag?: EnvironmentTag | '';
  sections?: ReportSection[];
};

export function normalizeSystemNames(values: string[] | undefined): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const raw of values ?? []) {
    const s = raw.trim();
    if (!s || seen.has(s)) continue;
    seen.add(s);
    out.push(s);
  }
  return out.sort((a, b) => a.localeCompare(b));
}

export function parseReportScopeSystems(scope: ReportScope): string[] {
  const fromList = normalizeSystemNames(scope.system_names);
  if (fromList.length > 0) return fromList;
  const legacy = scope.system_name?.trim();
  return legacy ? [legacy] : [];
}

export function normalizeReportSections(values: string[] | undefined): ReportSection[] {
  const out: ReportSection[] = [];
  const seen = new Set<string>();
  for (const raw of values ?? []) {
    const s = raw.trim() as ReportSection;
    if (!ALL_REPORT_SECTIONS.includes(s) || seen.has(s)) continue;
    seen.add(s);
    out.push(s);
  }
  return out;
}

/** Legacy reports without sections → all modules. */
export function effectiveReportSections(scope: ReportScope): ReportSection[] {
  const normalized = normalizeReportSections(scope.sections);
  if (normalized.length > 0) return normalized;
  return ALL_REPORT_SECTIONS;
}

export function initialSectionsForKind(kind: ReportKind): ReportSection[] {
  if (kind === 'daily') return [...DEFAULT_DAILY_SECTIONS];
  return [...ALL_REPORT_SECTIONS];
}

export function defaultSectionsForKind(kind: ReportKind): ReportSection[] {
  return initialSectionsForKind(kind);
}

export function parseReportScope(json?: string): ReportScope {
  if (!json?.trim() || json.trim() === '{}') return {};
  try {
    const v = JSON.parse(json) as ReportScope;
    if (!v || typeof v !== 'object') return {};
    const system_names = parseReportScopeSystems(v);
    return {
      ...v,
      system_names: system_names.length ? system_names : undefined,
      system_name: undefined,
      sections: normalizeReportSections(v.sections),
    };
  } catch {
    return {};
  }
}

export function formatReportScope(scope: ReportScope, kind?: ReportKind): string {
  const out: ReportScope = {};
  const names = parseReportScopeSystems(scope);
  const env = scope.environment_tag?.trim();
  if (names.length) out.system_names = names;
  if (env) out.environment_tag = env as EnvironmentTag;
  const sections =
    scope.sections && scope.sections.length > 0
      ? scope.sections
      : defaultSectionsForKind(kind ?? 'daily');
  if (sections?.length) out.sections = sections;
  if (!out.system_names?.length && !out.environment_tag && !out.sections?.length) return '{}';
  return JSON.stringify(out);
}

export function formatReportScopeSystemsLabel(
  scope: ReportScope,
  tr: (zh: string, en: string) => string,
): string {
  const names = parseReportScopeSystems(scope);
  if (names.length === 0) return '';
  if (names.length === 1) return names[0];
  if (names.length === 2) return `${names[0]}、${names[1]}`;
  return tr(`${names[0]}等${names.length}系统`, `${names[0]} +${names.length - 1} systems`);
}

export function uniqueSystemNames(items: { system_name?: string }[]): string[] {
  const set = new Set<string>();
  for (const item of items) {
    const s = item.system_name?.trim();
    if (s) set.add(s);
  }
  return [...set].sort((a, b) => a.localeCompare(b));
}

export type ReportListItem = {
  id: string;
  title: string;
  kind: ReportKind;
  status: ReportStatus;
  summary: string;
  period_start: string;
  period_end: string;
  generated_at?: string;
  created_at: string;
  schedule_id?: number; // cron dedup key; absent for run-now/manual reports
  task_id?: string; // owning-task back-ref (HLD-022), e.g. 'report-schedule:42'; set on scheduled + run-now
};

// --- ContentJSON shapes (mirror biz/report/content.go) ---

export type HeroStat = {
  key: string;
  label: string;
  value: number;
  unit?: string;
  delta_pct?: number;
  sparkline?: number[];
};

export type EntityRef = { key: string; name: string };

export type Paragraph = { text: string; entities?: EntityRef[] };

export type Narrative = { headline: string; paragraphs?: Paragraph[] };

export type KeyIncident = {
  id: number;
  title: string;
  severity: string;
  duration_min: number;
  status: string;
  root_cause_snippet?: string;
};

export type ToolCount = { tool: string; count: number };

export type ActionsSummary = {
  mutating_total: number;
  mutating_approved: number;
  safe_total: number;
  by_tool?: ToolCount[];
};

export type Advice = { text: string };

export type ResourceFacts = {
  available: boolean;
  cpu_avg: number;
  cpu_peak: number;
  mem_avg: number;
  mem_peak: number;
  disk_avg: number;
  disk_peak: number;
  net_rx_avg_bps?: number;
  net_rx_peak_bps?: number;
  net_tx_avg_bps?: number;
  net_tx_peak_bps?: number;
};

export type NetworkDeviceStat = {
  device_id: number;
  name?: string;
  online: boolean;
  net_rx_avg_bps?: number;
  net_rx_peak_bps?: number;
  net_tx_avg_bps?: number;
  net_tx_peak_bps?: number;
};

export type NetworkDeviceFacts = {
  available: boolean;
  devices?: NetworkDeviceStat[];
};

export type DeviceResourceStat = {
  device_id: number;
  name?: string;
  online: boolean;
  cpu_count?: number;
  mem_total_bytes?: number;
  disk_total_bytes?: number;
  cpu_avg?: number;
  cpu_peak?: number;
  mem_avg?: number;
  mem_peak?: number;
  disk_avg?: number;
  disk_peak?: number;
  net_rx_avg_bps?: number;
  net_rx_peak_bps?: number;
  net_tx_avg_bps?: number;
  net_tx_peak_bps?: number;
  downsize_suggest?: boolean;
  downsize_hint?: string;
};

export type DeviceResourceFacts = {
  available: boolean;
  devices?: DeviceResourceStat[];
};

export type FleetFacts = {
  total: number;
  online: number;
  roles?: Record<string, number>;
};

export type ChangeFact = {
  at: string;
  action: string;
  resource_type: string;
  resource_name?: string;
  actor?: string;
};

export type AssetFacts = {
  new_agents: number;
  new_skills: number;
  new_repos: number;
};

export type UsageFacts = {
  sessions: number;
  prompt_tokens: number;
  completion_tokens: number;
};

export type LogErrorSource = {
  device_id?: number;
  device_name?: string;
  kind: string;
  name: string;
  display_name?: string;
  ongrid_source?: string;
  count: number;
  sample_line?: string;
};

export type LogFacts = {
  available: boolean;
  total_errors: number;
  prev_total_errors?: number;
  delta_pct?: number;
  daily_sparkline?: number[];
  top_sources?: LogErrorSource[];
  query_pattern?: string;
  system_name?: string;
};

export type ReportContent = {
  version: string;
  hero: HeroStat[];
  narrative: Narrative;
  resource: ResourceFacts;
  fleet: FleetFacts;
  key_incidents?: KeyIncident[];
  actions_summary: ActionsSummary;
  changes?: ChangeFact[];
  /** @deprecated no longer collected or displayed in new reports */
  assets?: AssetFacts;
  /** @deprecated no longer collected or displayed in new reports */
  usage?: UsageFacts;
  logs: LogFacts;
  device_resources?: DeviceResourceFacts;
  network_devices?: NetworkDeviceFacts;
  advice?: Advice[];
  systems?: SystemContentBlock[];
  metadata?: {
    period_start?: string;
    period_end?: string;
    data_sources?: string[];
    sections?: ReportSection[];
    resource_mgmt?: ResourceMgmtConfig;
  };
};

export type SystemContentBlock = {
  system_name: string;
  narrative: Narrative;
  resource: ResourceFacts;
  fleet: FleetFacts;
  logs: LogFacts;
  device_resources?: DeviceResourceFacts;
  network_devices?: NetworkDeviceFacts;
  key_incidents?: KeyIncident[];
  actions_summary?: ActionsSummary;
  advice?: Advice[];
};

export type DeliveryResult = {
  channel_id: number;
  channel_type?: string;
  status: string;
  sent_at?: string;
  error?: string;
  fallback_used?: boolean;
};

export type ReportDetail = ReportListItem & {
  content?: ReportContent;
  content_md: string;
  timezone: string;
  schedule_id?: number;
  error_msg?: string;
  share_token?: string;
  delivery?: DeliveryResult[];
};

export type ReportSchedule = {
  id: number;
  name: string;
  description: string;
  kind: ReportKind;
  cron_spec: string;
  timezone: string;
  scope_json: string;
  channel_ids: number[];
  in_app_visible: boolean;
  agent_persona: string;
  prompt_override?: string;
  enabled: boolean;
  next_fire_at?: string;
  last_fire_at?: string;
  last_report_id?: string;
  created_at: string;
};

export type ScheduleInput = {
  name: string;
  description?: string;
  kind: ReportKind;
  cron_spec?: string;
  timezone?: string;
  scope_json?: string;
  channel_ids?: number[];
  in_app_visible?: boolean;
  prompt_override?: string;
};

// --- reports ---

export function listReports(params?: { status?: string; kind?: string; schedule_id?: number; task_id?: string; limit?: number; offset?: number }) {
  const q = new URLSearchParams();
  if (params?.status) q.set('status', params.status);
  if (params?.kind) q.set('kind', params.kind);
  if (params?.schedule_id != null) q.set('schedule_id', String(params.schedule_id));
  if (params?.task_id) q.set('task_id', params.task_id);
  if (params?.limit) q.set('limit', String(params.limit));
  if (params?.offset) q.set('offset', String(params.offset));
  const qs = q.toString();
  return request<{ reports: ReportListItem[] }>('GET', `/reports${qs ? `?${qs}` : ''}`);
}

export function getReport(id: string) {
  return request<ReportDetail>('GET', `/reports/${id}`);
}

export function deleteReport(id: string) {
  return request<void>('DELETE', `/reports/${id}`);
}

export function generateNow(body: { kind?: ReportKind; timezone?: string; scope_json?: string }) {
  return request<ReportDetail>('POST', '/reports', body);
}

export function shareReport(id: string) {
  return request<{ share_token: string; path: string }>('POST', `/reports/${id}/share`, {});
}

/** Public read of a shared report (no login). */
export async function getSharedReport(token: string): Promise<ReportDetail> {
  const resp = await fetch(`/api/r/${encodeURIComponent(token)}`, {
    headers: { Accept: 'application/json' },
  });
  const parsed = (await resp.json().catch(() => null)) as ReportDetail | { error?: string; code?: string } | null;
  if (!resp.ok) {
    const msg =
      parsed && typeof parsed === 'object' && 'error' in parsed && typeof parsed.error === 'string'
        ? parsed.error
        : `HTTP ${resp.status}`;
    throw new Error(msg);
  }
  return parsed as ReportDetail;
}

// --- schedules ---

export function listSchedules() {
  return request<{ schedules: ReportSchedule[] }>('GET', '/report-schedules');
}

export function getSchedule(id: number) {
  return request<ReportSchedule>('GET', `/report-schedules/${id}`);
}

export function createSchedule(body: ScheduleInput) {
  return request<ReportSchedule>('POST', '/report-schedules', body);
}

export function updateSchedule(id: number, body: ScheduleInput) {
  return request<ReportSchedule>('PUT', `/report-schedules/${id}`, body);
}

export function deleteSchedule(id: number) {
  return request<void>('DELETE', `/report-schedules/${id}`);
}

export function toggleSchedule(id: number, enabled: boolean) {
  return request<ReportSchedule>('POST', `/report-schedules/${id}/toggle`, { enabled });
}

export function runScheduleNow(id: number) {
  return request<ReportDetail>('POST', `/report-schedules/${id}/run-now`, {});
}

// --- report model settings ---

export type ResourceMgmtConfig = {
  enabled: boolean;
  cpu_peak_max_pct: number;
  mem_peak_max_pct: number;
  min_cpu_count: number;
  min_mem_gb: number;
};

export type ReportModelProvider = {
  id: string;
  label: string;
  models: string[];
  model?: string;
};

export type ReportModelConfig = {
  provider: string;
  model: string;
  use_platform_default: boolean;
  platform_default: { provider: string; model: string };
  providers: ReportModelProvider[];
};

export function getReportModel() {
  return request<ReportModelConfig>('GET', '/report-settings/model');
}

export function setReportModel(body: { provider: string; model: string }) {
  return request<ReportModelConfig>('PUT', '/report-settings/model', body);
}

export function getResourceMgmt() {
  return request<ResourceMgmtConfig>('GET', '/report-settings/resource-mgmt');
}

export function setResourceMgmt(body: ResourceMgmtConfig) {
  return request<ResourceMgmtConfig>('PUT', '/report-settings/resource-mgmt', body);
}
