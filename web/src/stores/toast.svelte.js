let toasts = $state([]);
let nextId = 0;

export function showToast(message, type = 'info') {
  const id = nextId++;
  toasts = [...toasts, { id, message, type }];
  setTimeout(() => {
    toasts = toasts.filter((t) => t.id !== id);
  }, 3000);
}

export function getToasts() {
  return toasts;
}
