import { Step } from '../types';

/**
 * Imperative (JS) implementation of the next-sibling (+) CSS combinator.
 *
 * @see {@link https://developer.mozilla.org/en-US/docs/Web/CSS/Next-sibling_combinator}
 */
export class NextSibling implements Step {
  run(input: Element[]) {
    const result = [];
    for (const element of input) {
      const nextSibling = element.nextElementSibling;
      if (nextSibling) {
        result.push(nextSibling);
      }
    }
    return result;
  }

  toString() {
    return 'NextSiblComb';
  }
}
