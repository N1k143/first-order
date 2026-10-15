import { ClosingSection } from "@/components/home/closing-section";
import { FaqSection } from "@/components/home/faq-section";
import { HeroSection } from "@/components/home/hero-section";
import { OverviewSection } from "@/components/home/overview-section";
import { WorkflowSection } from "@/components/home/workflow-section";

export default function HomePage() {
  return (
    <main id="main">
      <HeroSection />
      <OverviewSection />
      <WorkflowSection />
      <FaqSection />
      <ClosingSection />
    </main>
  );
}
