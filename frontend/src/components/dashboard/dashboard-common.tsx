"use client";

import { Check, Copy } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function MetricCard({
  icon: Icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: number;
  detail: string;
  tone: "emerald" | "blue" | "amber" | "violet";
}) {
  return (
    <Card className="overflow-hidden">
      <CardContent className="flex items-center justify-between p-4">
        <div className="min-w-0">
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="mt-1 text-3xl font-semibold">{value}</p>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {detail}
          </p>
        </div>
        <div
          className={cn(
            "flex h-11 w-11 items-center justify-center rounded-md",
            tone === "emerald" && "bg-emerald-500/12 text-emerald-600",
            tone === "blue" && "bg-blue-500/12 text-blue-600",
            tone === "amber" &&
              "bg-amber-500/14 text-amber-700 dark:text-amber-300",
            tone === "violet" &&
              "bg-violet-500/12 text-violet-600 dark:text-violet-300",
          )}
        >
          <Icon className="h-5 w-5" />
        </div>
      </CardContent>
    </Card>
  );
}

export function MiniStat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: number;
  icon: ComponentType<{ className?: string }>;
}) {
  return (
    <div className="rounded-md border border-border bg-muted/45 p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">{label}</p>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <p className="mt-1 text-xl font-semibold">{value}</p>
    </div>
  );
}

export function CommandLine({
  label,
  value,
  copied,
  onCopy,
}: {
  label: string;
  value: string;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <div className="grid gap-1">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className="grid grid-cols-[minmax(0,1fr)_36px] items-center gap-2">
        <code className="min-h-9 overflow-x-auto rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs leading-5 text-muted-foreground">
          {value}
        </code>
        <Button
          type="button"
          variant={copied ? "secondary" : "outline"}
          size="icon"
          title="Copy"
          onClick={onCopy}
        >
          {copied ? (
            <Check className="h-4 w-4" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  );
}

export function StatusBadge({ active }: { active: boolean }) {
  return (
    <Badge
      className={cn(
        active && "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300",
      )}
    >
      {active ? "live" : "idle"}
    </Badge>
  );
}

export function RoleBadge({ role }: { role: string }) {
  const label =
    role === "publisher+receiver"
      ? "publishing + receiving"
      : role === "publisher"
        ? "publishing"
        : role === "receiver"
          ? "receiving"
          : "connected";

  return (
    <Badge
      className={cn(
        role.includes("publisher") &&
          "bg-amber-500/14 text-amber-700 dark:text-amber-300",
        role.includes("receiver") &&
          "bg-blue-500/12 text-blue-700 dark:text-blue-300",
        role === "publisher+receiver" &&
          "bg-violet-500/12 text-violet-700 dark:text-violet-300",
      )}
    >
      {label}
    </Badge>
  );
}

export function LoadBar({ value, max }: { value: number; max: number }) {
  const width = Math.min(Math.round((value / Math.max(max, 1)) * 100), 100);
  return (
    <div className="h-1.5 overflow-hidden rounded-full bg-muted">
      <div
        className="h-full rounded-full bg-primary transition-all"
        style={{ width: `${width}%` }}
      />
    </div>
  );
}

export function EmptyState({ label }: { label: string }) {
  return (
    <div className="flex min-h-32 items-center justify-center px-3 py-8 text-center text-sm text-muted-foreground">
      {label}
    </div>
  );
}

export function FilterButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "h-8 rounded-md border border-border px-2 text-xs font-medium transition-colors",
        active
          ? "bg-primary text-primary-foreground"
          : "bg-background text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}
