import { PricingConfigPage, PricingPage } from "@/features/pricing/components/pricing-page";
import { RevenueAnalysisPage } from "@/features/pricing/components/revenue-analysis-page";

export function PricingRoute() {
  return <PricingPage />;
}

export function PricingConfigRoute() {
  return <PricingConfigPage />;
}

export function RevenueAnalysisRoute() {
  return <RevenueAnalysisPage />;
}
