import { Step } from '../types';

/**
 * Imperative (JS) implementation of the descendant ( ) CSS combinator
 *
 * @see {@link https://developer.mozilla.org/en-US/docs/Web/CSS/Descendant_combinator}
 */
export class Descendant implements Step {
  run(input: Element[]) {
    const descendantSet = new Set<Element>();
    for (const el of input) {
      const descendants = el.querySelectorAll('*');
      for (const el of descendants) {
        descendantSet.add(el);
      }
    }
    return Array.from(descendantSet);
  }

  toString() {
    return 'DescComb';
  }
}
