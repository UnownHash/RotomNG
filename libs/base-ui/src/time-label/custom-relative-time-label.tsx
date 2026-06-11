import { memo, useEffect, useMemo, useState } from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "../components/ui/tooltip";

const getCustomRelativeTime = (date1: number, date2: number) => {
  if (date1 === 0) {
    return "<never>";
  }

  const elapsed = date1 - date2;
  const absElapsed = Math.abs(elapsed);

  // If difference is 0 or very small (less than 1 second), return 'now'
  if (absElapsed < 1000) {
    return "now";
  }

  const days = Math.floor(absElapsed / (24 * 60 * 60 * 1000));
  const hours = Math.floor(
    (absElapsed % (24 * 60 * 60 * 1000)) / (60 * 60 * 1000),
  );
  const minutes = Math.floor((absElapsed % (60 * 60 * 1000)) / (60 * 1000));
  const seconds = Math.floor((absElapsed % (60 * 1000)) / 1000);

  let timeString = "";
  if (days > 0) timeString += `${days}d`;
  if (hours > 0) timeString += `${hours}h`;
  if (minutes > 0) timeString += `${minutes}m`;
  timeString += `${seconds}s`;

  if (elapsed < 0) {
    return `${timeString} ago`;
  } else {
    return `in ${timeString}`;
  }
};

const CustomRelativeTimeLabelComponent = ({
  timestamp,
}: {
  timestamp: number;
}) => {
  const [currentTime, setCurrentTime] = useState(Date.now());

  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  const relativeTime = useMemo(
    () => getCustomRelativeTime(timestamp, currentTime),
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
export const CustomRelativeTimeLabel = memo(CustomRelativeTimeLabelComponent);
