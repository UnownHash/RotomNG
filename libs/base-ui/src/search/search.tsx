import { Search as SearchIcon, X } from "lucide-react";
import type React from "react";
import { memo, useCallback } from "react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { cn } from "../lib/utils";

interface SearchProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}

const SearchComponent: React.FC<SearchProps> = ({
  value,
  onChange,
  placeholder = "Search devices...",
  className,
}) => {
  const handleClear = useCallback(() => {
    onChange("");
  }, [onChange]);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(e.target.value);
    },
    [onChange],
  );

  return (
    <div className={cn("relative mb-2 w-full", className)}>
      <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        value={value}
        onChange={handleChange}
        placeholder={placeholder}
        className="border-border/40 bg-card/40 pr-10 pl-10 backdrop-blur-xl focus-visible:shadow-(--glow-sm) focus-visible:ring-(--brand-1)"
      />
      {value && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="clear search"
          onClick={handleClear}
          className="absolute right-1 top-1/2 size-7 -translate-y-1/2"
        >
          <X className="size-4" />
        </Button>
      )}
    </div>
  );
};

// Export memoized component
export const Search = memo(SearchComponent);
