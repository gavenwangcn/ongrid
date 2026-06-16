export type EnvironmentTag = 'dev' | 'test' | 'prod';

export type EnvironmentFilterValue = '' | EnvironmentTag;

export const ENVIRONMENT_TAGS: EnvironmentTag[] = ['dev', 'test', 'prod'];

export const ENVIRONMENT_TAG_LABELS: Record<EnvironmentTag, string> = {
  dev: '开发',
  test: '测试',
  prod: '生产',
};

export const ENVIRONMENT_TAG_LABELS_EN: Record<EnvironmentTag, string> = {
  dev: 'Development',
  test: 'Testing',
  prod: 'Production',
};

export function isEnvironmentTag(v: string): v is EnvironmentTag {
  return ENVIRONMENT_TAGS.includes(v as EnvironmentTag);
}

export function matchesEnvironmentFilter(
  tag: string | undefined,
  filter: EnvironmentFilterValue,
): boolean {
  if (!filter) return true;
  return tag === filter;
}

export function environmentTagLabel(tag: string | undefined, tr: (zh: string, en: string) => string): string {
  if (!tag?.trim()) return '—';
  switch (tag) {
    case 'dev':
      return tr(ENVIRONMENT_TAG_LABELS.dev, ENVIRONMENT_TAG_LABELS_EN.dev);
    case 'test':
      return tr(ENVIRONMENT_TAG_LABELS.test, ENVIRONMENT_TAG_LABELS_EN.test);
    case 'prod':
      return tr(ENVIRONMENT_TAG_LABELS.prod, ENVIRONMENT_TAG_LABELS_EN.prod);
    default:
      return tag;
  }
}
