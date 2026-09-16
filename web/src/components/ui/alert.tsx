import * as React from "react"
import { cn } from "cn"

function Alert({
  className,
  variant = "default",
  ...props
}: React.ComponentProps<"div"> & { variant?: "default" | "destructive" | "warning" }) {
  return (
    <div
      data-slot="alert"
      data-variant={variant}
      className={cn(
        "relative flex w-full items-start gap-3 rounded-lg border p-3 text-sm",
        variant === "default" && "bg-muted/50 text-foreground",
        variant === "destructive" && "border-destructive/30 bg-destructive/10 text-destructive",
        variant === "warning" && "border-yellow-400/30 bg-yellow-50 text-yellow-800",
        className
      )}
      {...props}
    />
  )
}

function AlertTitle({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="alert-title"
      className={cn("font-medium leading-snug", className)}
      {...props}
    />
  )
}

function AlertDescription({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="alert-description"
      className={cn("text-sm leading-snug opacity-80", className)}
      {...props}
    />
  )
}

export { Alert, AlertTitle, AlertDescription }