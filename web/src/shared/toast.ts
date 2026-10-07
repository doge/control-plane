export type ToastKind = "success" | "error";
export type ToastMessage = { message: string; kind: ToastKind };

const listeners = new Set<(toast: ToastMessage) => void>();
const pending: ToastMessage[] = [];

/** Publish a toast notification to every mounted toast region. */
export function showToast(message: string, kind: ToastKind = "error") {
  const toast = { message, kind };
  if (listeners.size === 0) pending.push(toast);
  else listeners.forEach((listener) => listener(toast));
}

/** Subscribe to new toasts and return a function that removes the listener. */
export function subscribeToToasts(listener: (toast: ToastMessage) => void) {
  listeners.add(listener);
  pending.splice(0).forEach(listener);
  return () => {
    listeners.delete(listener);
  };
}
