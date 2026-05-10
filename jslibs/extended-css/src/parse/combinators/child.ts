import { Step } from '../types';

/**
 * Imperative (JS) implementation of the child (>) CSS combinator.
 *
 * @see {@link https://developer.mozilla.org/en-US/docs/Web/CSS/Child_combinator}
 */
export class Child implements Step {
  run(input: Element[]) {
    const res = [];
    for (const el of input) {
      res.push(...el.children);
    }
    return res;
  }

  toString() {
    return 'ChildComb';
  }
}
