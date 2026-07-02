import { useEffect, useState } from "react";

export function useEventStream(url?: string) {
  const [lines, setLines] = useState<string[]>([]);

  useEffect(() => {
    if (!url) return;
    setLines([]);
    const source = new EventSource(url);
    source.onmessage = (event) => {
      setLines((current) => [...current.slice(-499), event.data]);
    };
    source.onerror = () => {
      source.close();
    };
    return () => source.close();
  }, [url]);

  return lines;
}
