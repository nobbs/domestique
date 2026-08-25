import { IconLoader, type IconProps } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

function Spinner({ className, ...props }: IconProps) {
  return (
    <IconLoader
      data-slot="spinner"
      role="status"
      aria-label="Loading"
      className={cn("size-4 animate-spin", className)}
      {...props}
    />
  );
}

export { Spinner };
