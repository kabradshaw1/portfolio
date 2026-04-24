"use client";

import {
  ShoppingCart,
  Package,
  CreditCard,
  CheckCircle,
  XCircle,
  Clock,
  Undo2,
  type LucideIcon,
} from "lucide-react";
import type { TimelineEvent } from "@/lib/go-projector-api";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

interface StepStyle {
  icon: LucideIcon;
  color: string;
  bgColor: string;
  label: string;
}

function getStepStyle(event: TimelineEvent): StepStyle {
  switch (event.eventType) {
    case "order.created":
      return {
        icon: ShoppingCart,
        color: "text-blue-500",
        bgColor: "bg-blue-500/10",
        label: "Order Created",
      };
    case "order.stock_reserved":
      return {
        icon: Package,
        color: "text-blue-500",
        bgColor: "bg-blue-500/10",
        label: "Stock Reserved",
      };
    case "order.payment_initiated":
      return {
        icon: CreditCard,
        color: "text-blue-500",
        bgColor: "bg-blue-500/10",
        label: "Payment Initiated",
      };
    case "order.payment_completed":
      return {
        icon: CreditCard,
        color: "text-green-500",
        bgColor: "bg-green-500/10",
        label: "Payment Completed",
      };
    case "order.completed":
      return {
        icon: CheckCircle,
        color: "text-green-500",
        bgColor: "bg-green-500/10",
        label: "Order Completed",
      };
    case "order.failed":
      return {
        icon: XCircle,
        color: "text-red-500",
        bgColor: "bg-red-500/10",
        label: "Order Failed",
      };
    case "order.cancelled":
      return {
        icon: XCircle,
        color: "text-red-500",
        bgColor: "bg-red-500/10",
        label: "Order Cancelled",
      };
    case "order.stock_released":
      return {
        icon: Undo2,
        color: "text-yellow-500",
        bgColor: "bg-yellow-500/10",
        label: "Stock Released",
      };
    case "order.payment_refunded":
      return {
        icon: Undo2,
        color: "text-yellow-500",
        bgColor: "bg-yellow-500/10",
        label: "Payment Refunded",
      };
    default:
      return {
        icon: Clock,
        color: "text-muted-foreground",
        bgColor: "bg-muted",
        label: event.eventType,
      };
  }
}

function getDescription(event: TimelineEvent): string | null {
  const data = event.data;
  if (!data) return null;

  switch (event.eventType) {
    case "order.created": {
      const itemCount = Array.isArray(data.items)
        ? data.items.length
        : undefined;
      const total =
        typeof data.totalCents === "number"
          ? formatPrice(data.totalCents)
          : undefined;
      if (itemCount !== undefined && total) {
        return `${itemCount} item${itemCount !== 1 ? "s" : ""}, ${total}`;
      }
      return null;
    }
    case "order.failed":
    case "order.cancelled":
      return typeof data.reason === "string" ? data.reason : null;
    case "order.payment_completed":
      return typeof data.paymentId === "string"
        ? `Payment ${data.paymentId.slice(0, 8)}...`
        : null;
    default:
      return null;
  }
}

interface OrderTimelineProps {
  events: TimelineEvent[];
}

export default function OrderTimeline({ events }: OrderTimelineProps) {
  if (events.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No timeline events yet.</p>
    );
  }

  return (
    <div className="relative space-y-0">
      {events.map((event, idx) => {
        const style = getStepStyle(event);
        const Icon = style.icon;
        const description = getDescription(event);
        const isLast = idx === events.length - 1;

        return (
          <div key={event.eventId} className="relative flex gap-4 pb-8">
            {/* Vertical connector line */}
            {!isLast && (
              <div className="absolute left-[17px] top-[36px] bottom-0 w-px bg-foreground/10" />
            )}

            {/* Icon circle */}
            <div
              className={`relative z-10 flex size-9 shrink-0 items-center justify-center rounded-full ${style.bgColor}`}
            >
              <Icon className={`size-4 ${style.color}`} />
            </div>

            {/* Content */}
            <div className="flex-1 pt-1">
              <p className="text-sm font-medium">{style.label}</p>
              {description && (
                <p className="mt-0.5 text-sm text-muted-foreground">
                  {description}
                </p>
              )}
              <p className="mt-1 text-xs text-muted-foreground">
                {new Date(event.timestamp).toLocaleString()}
              </p>
            </div>
          </div>
        );
      })}
    </div>
  );
}
