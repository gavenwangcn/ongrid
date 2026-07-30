import { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, X } from 'lucide-react';
import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';

type Props = {
  options: string[];
  value: string[];
  onChange(next: string[]): void;
};

/** Tag-style multi-select dropdown for report system scope. Empty = all systems. */
export function ReportSystemsPicker({ options, value, onChange }: Props) {
  const { tr } = useI18n();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selected = new Set(value);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  const toggle = (name: string) => {
    if (selected.has(name)) {
      onChange(value.filter((s) => s !== name));
      return;
    }
    onChange([...value, name].sort((a, b) => a.localeCompare(b)));
  };

  const removeTag = (name: string) => {
    onChange(value.filter((s) => s !== name));
  };

  return (
    <div ref={rootRef} className="relative">
      <div
        className={cn(
          'flex min-h-[34px] w-full cursor-pointer items-center gap-1 rounded-md border bg-zinc-950 px-2 py-1.5 text-sm transition-colors',
          open ? 'border-indigo-500/50 ring-1 ring-indigo-500/20' : 'border-zinc-700 hover:border-zinc-600',
        )}
        onClick={() => setOpen((v) => !v)}
        role="combobox"
        aria-expanded={open}
      >
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
          {value.length === 0 ? (
            <span className="text-xs text-zinc-400">{tr('全部系统', 'All systems')}</span>
          ) : (
            value.map((name) => (
              <span
                key={name}
                className="inline-flex max-w-full items-center gap-0.5 rounded bg-indigo-500/15 px-1.5 py-0.5 text-[11px] text-indigo-200"
              >
                <span className="truncate">{name}</span>
                <button
                  type="button"
                  className="rounded p-0.5 hover:bg-indigo-500/25"
                  aria-label={tr('移除', 'Remove')}
                  onClick={(e) => {
                    e.stopPropagation();
                    removeTag(name);
                  }}
                >
                  <X size={10} />
                </button>
              </span>
            ))
          )}
        </div>
        <ChevronDown
          size={14}
          className={cn('shrink-0 text-zinc-500 transition-transform', open && 'rotate-180')}
        />
      </div>

      {open && (
        <div className="absolute z-50 mt-1 max-h-52 w-full overflow-y-auto rounded-md border border-zinc-700 bg-zinc-900 py-1 shadow-lg">
          <button
            type="button"
            className={cn(
              'flex w-full items-center justify-between px-2.5 py-1.5 text-left text-xs hover:bg-zinc-800/80',
              value.length === 0 ? 'text-indigo-200' : 'text-zinc-300',
            )}
            onClick={() => {
              onChange([]);
            }}
          >
            <span>{tr('全部系统', 'All systems')}</span>
            {value.length === 0 && <Check size={12} className="text-indigo-400" />}
          </button>
          {options.length === 0 ? (
            <div className="px-2.5 py-2 text-[11px] text-zinc-600">
              {tr('暂无系统名称', 'No system names yet')}
            </div>
          ) : (
            options.map((name) => {
              const on = selected.has(name);
              return (
                <button
                  key={name}
                  type="button"
                  className={cn(
                    'flex w-full items-center justify-between px-2.5 py-1.5 text-left text-xs hover:bg-zinc-800/80',
                    on ? 'text-indigo-200' : 'text-zinc-300',
                  )}
                  onClick={() => toggle(name)}
                >
                  <span className="truncate">{name}</span>
                  {on && <Check size={12} className="shrink-0 text-indigo-400" />}
                </button>
              );
            })
          )}
        </div>
      )}

      {value.length > 0 && (
        <p className="mt-1 text-[11px] text-zinc-600">
          {tr(
            '已选系统将分别输出一段报告内容，不会合并汇总。',
            'Each selected system gets its own report section — no cross-system merge.',
          )}
        </p>
      )}
    </div>
  );
}
