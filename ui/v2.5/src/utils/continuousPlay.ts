import * as GQL from "src/core/generated-graphql";
import { queryFindScenes } from "src/core/StashService";
import { ListFilterModel } from "src/models/list-filter/filter";
import { PerformersCriterion } from "src/models/list-filter/criteria/performers";
import { RandomExcludedCriterion } from "src/models/list-filter/criteria/random-excluded";
import { StudiosCriterion } from "src/models/list-filter/criteria/studios";
import SceneQueue from "src/models/sceneQueue";
import { INamedObject } from "./navigation";

export type ContinuousPlayCategory =
  | { studio: INamedObject }
  | { performer: INamedObject };

function makeContinuousPlayFilter(category: ContinuousPlayCategory) {
  const filter = new ListFilterModel(GQL.FilterMode.Scenes, undefined);
  filter.itemsPerPage = 40;
  // assigns a fresh random seed, giving a stable shuffled order for the
  // life of this queue rather than a fresh pick on every scene
  filter.sortBy = "random";

  if ("studio" in category) {
    const criterion = new StudiosCriterion();
    criterion.value = {
      items: [
        {
          id: category.studio.id,
          label: category.studio.name || `Studio ${category.studio.id}`,
        },
      ],
      excluded: [],
      depth: 0,
    };
    filter.criteria.push(criterion);
  } else {
    const criterion = new PerformersCriterion();
    criterion.value.items = [
      {
        id: category.performer.id,
        label:
          category.performer.name || `Performer ${category.performer.id}`,
      },
    ];
    filter.criteria.push(criterion);
  }

  const excludeRandomExcluded = new RandomExcludedCriterion();
  excludeRandomExcluded.value = "false";
  filter.criteria.push(excludeRandomExcluded);

  return filter;
}

// Builds a randomized, category-scoped scene queue -- honouring each
// scene's per-user "exclude from random" flag -- and returns a link to its
// first scene with autoplay and continue-playlist enabled. Playback beyond
// the first scene is handled entirely by the existing SceneQueue /
// continuePlaylist machinery in Scene.tsx: it's the same queue, just
// pre-built and pre-configured to start playing immediately.
export async function startContinuousPlay(
  category: ContinuousPlayCategory
): Promise<string | undefined> {
  const filter = makeContinuousPlayFilter(category);
  const result = await queryFindScenes(filter);
  const firstScene = result.data?.findScenes.scenes[0];
  if (!firstScene) return undefined;

  const queue = SceneQueue.fromListFilterModel(filter);
  return queue.makeLink(firstScene.id, { autoPlay: true, continue: true });
}
