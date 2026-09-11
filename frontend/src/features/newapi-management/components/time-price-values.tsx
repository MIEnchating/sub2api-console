import { pricePerMillion } from "../lib/pricing-number";

export function TimePriceValues(props: { peak: string; offPeak: string }) {
  return (
    <div className="flex flex-col gap-1 whitespace-nowrap">
      <span>高峰 {pricePerMillion(props.peak)}</span>
      <span>空闲 {pricePerMillion(props.offPeak)}</span>
    </div>
  );
}
