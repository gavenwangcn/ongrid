import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';

type Props = {
  options: string[];
  value: string[];
  onChange(next: string[]): void;
};

/** Multi-select business systems for report scope. Empty selection = all systems. */
export function ReportSystemsPicker({ options, value, onChange }: Props) {
  const { tr } = useI18n();
  const selected = new Set(value);

  const toggle = (name: string) => {
    if (selected.has(name)) {
      onChange(value.filter((s) => s !== name));
      return;
    }
    onChange([...value, name].sort((a, b) => a.localeCompare(b)));
  };

  const selectAll = () => onChange([]);
  const allSelected = value.length === 0;

  return (
    <div className="space-y-1.5">
      <label
        className={cn(
          'flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-2 transition-colors',
          allSelected ? 'border-indigo-500/50 bg-indigo-500/10' : 'border-zinc-800 bg-zinc-950/40 hover:border-zinc-700',
        )}
      >
        <input
          type="checkbox"
          className="accent-indigo-500"
          checked={allSelected}
          onChange={selectAll}
        />
        <span className="text-xs font-medium text-zinc-200">{tr('全部系统', 'All systems')}</span>
      </label>
      {options.map((name) => {
        const on = selected.has(name);
        return (
          <label
            key={name}
            className={cn(
              'flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-2 transition-colors',
              on ? 'border-indigo-500/50 bg-indigo-500/10' : 'border-zinc-800 bg-zinc-950/40 hover:border-zinc-700',
            )}
          >
            <input
              type="checkbox"
              className="accent-indigo-500"
              checked={on}
              onChange={() => toggle(name)}
            />
            <span className="text-xs text-zinc-200">{name}</span>
          </label>
        );
      })}
      {value.length > 0 && (
        <p className="text-[11px] text-zinc-600">
          {tr(
            '已选系统将分别输出一段报告内容，不会合并汇总。',
            'Each selected system gets its own report section — no cross-system merge.',
          )}
        </p>
      )}
    </div>
  );
}
