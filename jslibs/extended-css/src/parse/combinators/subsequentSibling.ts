import { Step } from '../types';

/**
 * Imperative (JS) implementation of the subsequent-sibling (~) CSS combinator.
 *
 * @see {@link https://developer.mozilla.org/en-US/docs/Web/CSS/Subsequent-sibling_combinator}
 */
export class SubsequentSibling implements Step {
  run(input: Element[]) {
    const result = new Set<Element>();

    for (const el of input) {
      let sib = el.nextElementSibling;
      while (sib) {
        result.add(sib);
        sib = sib.nextElementSibling;
      }
    }

    return Array.from(result);
  }

  toString() {
    return 'SubsSiblComb';
  }
}
