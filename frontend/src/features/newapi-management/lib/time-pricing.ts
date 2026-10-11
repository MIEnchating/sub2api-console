import type { ModelTimePricing, Sub2APIModelPrice } from "@/api";
import { pricePerMillion } from "./pricing-number";

export function timePricingDescription(schedule: ModelTimePricing): string {
  const days = schedule.weekdays_only ? "周一至周五" : "每天";
  const periods = schedule.periods
    .map((period) => `${period.start_time}–${period.end_time}`)
    .join("、");
  return `${days} ${periods}（${schedule.timezone}），其余时间为空闲`;
}

function minutes(value: string): number {
  const parts = value.split(":");
  return Number(parts[0]) * 60 + Number(parts[1]);
}

function timeBoundary(zone: string, value: string, edge: "start" | "end"): string {
  const totalMinutes = minutes(value);
  const hour = Math.floor(totalMinutes / 60);
  const minute = totalMinutes % 60;
  const hourProbe = `hour(${zone})`;
  if (edge === "start") {
    if (minute === 0) return `${hourProbe} >= ${hour}`;
    return `(${hourProbe} > ${hour} || (${hourProbe} == ${hour} && minute(${zone}) >= ${minute}))`;
  }
  if (minute === 0) return `${hourProbe} < ${hour}`;
  return `(${hourProbe} < ${hour} || (${hourProbe} == ${hour} && minute(${zone}) < ${minute}))`;
}

function ratesExpression(rates: ModelTimePricing["peak"]): string {
  return `p * ${pricePerMillion(rates.input_price)} + c * ${pricePerMillion(rates.output_price)} + cr * ${pricePerMillion(rates.cache_read_price)}`;
}

export function timePricingExpression(price: Sub2APIModelPrice): string {
  const schedule = price.time_pricing;
  if (!schedule) return "";
  const zone = JSON.stringify(schedule.timezone);
  const periods = schedule.periods
    .map(
      (period) =>
        `(${timeBoundary(zone, period.start_time, "start")} && ${timeBoundary(zone, period.end_time, "end")})`,
    )
    .join(" || ");
  let condition = `(${periods})`;
  if (schedule.weekdays_only) {
    condition = `(weekday(${zone}) >= 1 && weekday(${zone}) <= 5) && ${condition}`;
  }
  const base = ratesExpression({
    input_price: price.input_price,
    output_price: price.output_price,
    cache_read_price: price.cache_read_price ?? "0",
  });
  return `${condition} ? tier("高峰", ${ratesExpression(schedule.peak)}) : tier("空闲", ${base})`;
}
