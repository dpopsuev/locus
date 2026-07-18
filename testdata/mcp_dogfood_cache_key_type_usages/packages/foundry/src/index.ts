import type { DiscussionRef } from "@alef/kernel";
import { makeRef } from "@alef/kernel";

export type FoundryOpts = {
  discussion?: DiscussionRef;
};

export function withDiscussion(title: string): FoundryOpts {
  return { discussion: makeRef("forum", "topic", title) };
}
