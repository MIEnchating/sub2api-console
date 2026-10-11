const dateOptions: Intl.DateTimeFormatOptions = {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
};
const minuteFormatter = new Intl.DateTimeFormat("zh-CN", dateOptions);
const secondFormatter = new Intl.DateTimeFormat("zh-CN", { ...dateOptions, second: "2-digit" });
const fullFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "numeric",
  day: "numeric",
  hour: "numeric",
  minute: "numeric",
  second: "numeric",
  hour12: false,
});

export function formatDateTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : fullFormatter.format(date);
}

export function formatDate(value: string | null | undefined, includeSeconds = false): string {
  if (!value) return "未知时间";
  const numeric = /^\d{10,13}$/.test(value) ? Number(value) : null;
  let dateInput: string | number = value;
  if (numeric !== null) dateInput = value.length === 10 ? numeric * 1000 : numeric;
  const date = new Date(dateInput);
  if (Number.isNaN(date.getTime())) return value;
  return (includeSeconds ? secondFormatter : minuteFormatter).format(date);
}
