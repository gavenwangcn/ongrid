import { request } from './client';

// Reports API (HLD-014). Scheduled operational reports — list + detail
// + manual generate + schedule CRUD. Backend routes under /v1/reports
// and /v1/report-schedules.

import type { EnvironmentTag } from './environment';

export type ReportStatus = 'pending' | 'generating' | 'ready' | 'failed';
export type ReportKind = 'daily' | 'weekly' | 'monthly' | 'yearly' | 'custom';

export type ReportScope = {
  system_name?: string;
  environment_tag?: EnvironmentTag | '';
};

export function parseReportScope(json?: string): ReportScope {
  if (!json?.trim() || json.trim() === '{}') return {};
  try {
    const v = JSON.parse(json) as ReportScope;
    return v && typeof v === 'object' ? v : {};
  } catch {
    return {};
  }
}

export function formatReportScope(scope: ReportScope): string {
  const out: ReportScope = {};
  const name = scope.system_name?.trim();
  const env = scope.environment_tag?.trim();
  if (name) out.system_name = name;
  if (env) out.environment_tag = env as EnvironmentTag;
  if (!out.system_name && !out.environment_tag) return '{}';
  return JSON.stringify(out);
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

export function listReports(params?: { status?: string; kind?: string; limit?: number; offset?: number }) {
  const q = new URLSearchParams();
  if (params?.status) q.set('status', params.status);
  if (params?.kind) q.set('kind', params.kind);
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
