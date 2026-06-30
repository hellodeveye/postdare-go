import { useMemo } from "react";

import { cn } from "../lib/utils";

export function LogViewer({ log, liveLines, className }: { log?: string; liveLines?: string[]; className?: string }) {
  const text = useMemo(() => {
    const staticLines = log ? log.trimEnd().split("\n") : [];
    return [...staticLines, ...(liveLines ?? [])].slice(-700);
  }, [log, liveLines]);

  return (
    <div className={cn("log-scroll h-[420px] overflow-auto rounded-lg border border-border bg-black p-3 font-mono text-[13px] leading-6 text-zinc-100", className)}>
      {text.length ? (
        text.map((line, index) => (
          <div key={`${index}-${line}`} className="whitespace-pre-wrap break-words">
            <span className="select-none pr-3 text-zinc-500">{String(index + 1).padStart(3, "0")}</span>
            {line}
          </div>
        ))
      ) : (
        <div className="text-zinc-500">No log output</div>
      )}
    </div>
  );
}
