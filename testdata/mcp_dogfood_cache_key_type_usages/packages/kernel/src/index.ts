/** Ambient discussion coordinates — mirrors alef kernel DiscussionRef. */
export interface DiscussionRef {
  forumId: string;
  topicId: string;
  topicTitle: string;
}

export function makeRef(forumId: string, topicId: string, topicTitle: string): DiscussionRef {
  return { forumId, topicId, topicTitle };
}
