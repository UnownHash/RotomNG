import { memo, useEffect, useMemo, useState } from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "../components/ui/tooltip";

const relativeTimeFormat = new Intl.RelativeTimeFormat("en", {
  style: "narrow",
  numeric: "auto",
});

const units = {
  year: 24 * 60 * 60 * 1000 * 365,
  month: (24 * 60 * 60 * 1000 * 365) / 12,
  day: 24 * 60 * 60 * 1000,
  hour: 60 * 60 * 1000,
  minute: 60 * 1000,
  second: 1000,
};

const getRelativeTime = (date1: number, date2: number) => {
  if (date1 === 0) {
    return "<never>";
  }

  const elapsed = date1 - date2;

  // "Math.abs" accounts for both "past" & "future" scenarios
  for (const u in units) {
    const unit = u as keyof typeof units;

    if (Math.abs(elapsed) > units[unit]) {
      return relativeTimeFormat.format(Math.round(elapsed / units[unit]), unit);
    }
  }

  return relativeTimeFormat.format(
    Math.round(elapsed / units.second),
    "second",
  );
};

const RelativeTimeLabelComponent = ({ timestamp }: { timestamp: number }) => {
  const [currentTime, setCurrentTime] = useState(Date.now());

  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  const relativeTime = useMemo(
    () => getRelativeTime(timestamp, currentTime),
    [timestamp, currentTime],
  );

  const tooltipContent = useMemo(
    () => new Date(timestamp).toLocaleString(),
    [timestamp],
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span>{relativeTime}</span>
      </TooltipTrigger>
      <TooltipContent>{tooltipContent}</TooltipContent>
    </Tooltip>
  );
};

// Export memoized component
export const RelativeTimeLabel = memo(RelativeTimeLabelComponent);
