// Semantic colors shared by status badges and result summaries.
export const statusToneClasses = {
  success: "border-success/30 bg-success/10 text-success",
  warning: "border-warning/30 bg-warning/10 text-warning",
  danger: "border-destructive/30 bg-destructive/10 text-destructive",
  info: "border-info/30 bg-info/10 text-info",
  purple:
    "border-purple-600/30 bg-purple-600/10 text-purple-700 dark:border-purple-400/30 dark:bg-purple-400/10 dark:text-purple-300",
  neutral: "border-border bg-muted/30 text-muted-foreground",
} as const;

export type StatusTone = keyof typeof statusToneClasses;
