import { apiFetch } from "@/lib/api/client";
import type { AlertRule, AlertRuleInput, NotificationList } from "@/lib/alerts";

export function listAlertRules(): Promise<{ rules: AlertRule[] }> {
  return apiFetch("/alert-rules");
}

export function createAlertRule(input: AlertRuleInput): Promise<AlertRule> {
  return apiFetch("/alert-rules", { method: "POST", body: input });
}

export function updateAlertRule(id: string, input: AlertRuleInput): Promise<AlertRule> {
  return apiFetch(`/alert-rules/${id}`, { method: "PUT", body: input });
}

export function deleteAlertRule(id: string): Promise<void> {
  return apiFetch(`/alert-rules/${id}`, { method: "DELETE" });
}

export function listNotifications(): Promise<NotificationList> {
  return apiFetch("/notifications");
}

export function markNotificationsRead(): Promise<void> {
  return apiFetch("/notifications/read", { method: "POST" });
}
