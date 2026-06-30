import { useLayoutEffect, useMemo, useRef } from "react";

import { cn } from "../lib/utils";

export function LogViewer({ log, liveLines, className }: { log?: string; liveLines?: string[]; className?: string }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldStickToBottomRef = useRef(true);
  const text = useMemo(() => {
    const staticLines = log ? log.trimEnd().split("\n") : [];
    return [...staticLines, ...(liveLines ?? [])].slice(-700);
  }, [log, liveLines]);
  const textKey = `${text.length}:${text[text.length - 1] ?? ""}`;

  useLayoutEffect(() => {
    const container = scrollRef.current;
    if (!container || !shouldStickToBottomRef.current) return;
    container.scrollTop = container.scrollHeight;
  }, [textKey]);

  const handleScroll = () => {
    const container = scrollRef.current;
    if (!container) return;
    const distanceFromBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
    shouldStickToBottomRef.current = distanceFromBottom < 48;
  };

  return (
    <div ref={scrollRef} onScroll={handleScroll} className={cn("log-scroll h-[420px] overflow-auto rounded-lg border border-border bg-black p-3 font-mono text-[13px] leading-6 text-zinc-100", className)}>
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
