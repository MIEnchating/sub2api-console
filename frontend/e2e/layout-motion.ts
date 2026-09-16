import { expect, type Locator } from "@playwright/test";

export async function waitForLayoutAnimations(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  await expect
    .poll(() =>
      locator.evaluate((element) => {
        const animations = new Set(element.getAnimations({ subtree: true }));
        for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
          for (const animation of ancestor.getAnimations()) animations.add(animation);
        }
        return [...animations].filter(
          (animation) =>
            (animation.pending || animation.playState === "running") &&
            animation.effect?.getComputedTiming().iterations !== Infinity,
        ).length;
      }),
    )
    .toBe(0);
}
