import { CriterionModifier } from "src/core/generated-graphql";
import { BooleanCriterion, ModifierCriterionOption } from "./criterion";

// Not shown in the criterion-editor UI -- this exists so the
// Continuous Play queue filter (src/utils/continuousPlay.ts) can exclude
// random_excluded scenes via the ordinary ListFilterModel/SceneQueue path.
export const RandomExcludedCriterionOption = new ModifierCriterionOption({
  messageID: "random_excluded",
  type: "random_excluded",
  modifierOptions: [],
  defaultModifier: CriterionModifier.Equals,
  options: ["true", "false"],
  makeCriterion: () => new RandomExcludedCriterion(),
  hidden: true,
});

export class RandomExcludedCriterion extends BooleanCriterion {
  constructor() {
    super(RandomExcludedCriterionOption);
  }
}
