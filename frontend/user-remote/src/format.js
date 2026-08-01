/**
 * Relative time formatting (Chinese UI copy).
 * <60s 刚刚 / <60min n 分钟前 / <24h n 小时前 / <7d n 天前 / else YYYY-MM-DD
 * @param {number} ms created_at in unix milliseconds
 * @returns {string}
 */
export function formatRelativeTime(ms) {
  if (!ms || Number.isNaN(ms)) return '';

  const diff = Date.now() - ms;
  if (diff < 60_000) return '刚刚';

  const minutes = Math.floor(diff / 60_000);
  if (minutes < 60) return `${minutes} 分钟前`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;

  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} 天前`;

  const d = new Date(ms);
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
