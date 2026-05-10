import { SelectorList } from '../parse/types';

export interface ExecuteOptions {
  /**
   * If true, a failing selector (error at any step) is ignored
   * and execution continues with the next selector. Default: false.
   *
   * This option is not semantically equivalent to `<forgiving-selector-list>`, which
   * ignores **parsing** errors. However, in practice css-tree is quite lenient
   * except for the most explicit syntax errors, so this should be
   * sufficient to approximate the same behavior — applied during evaluation instead.
   *
   * @see {@link https://drafts.csswg.org/selectors/#forgiving-selector}
   */
  forgiving?: boolean;
}

/**
 * Executes a parsed selector list against a set of input elements.
 *
 * Applies each selector step-by-step and returns the unique set of
 * elements that match any of the selectors in the list.
 */
export class SelectorExecutor {
  constructor(private query: SelectorList) {}

  match(input: Element | Element[], options: ExecuteOptions = {}): Element[] {
    const elements: Set<Element> = new Set();

    const initialEls = Array.isArray(input) ? input : [input];
    for (const selector of this.query) {
      try {
        let els = initialEls;
        for (const step of selector) {
          els = step.run(els);
          if (els.length === 0) break;
        }
        els.forEach((el) => elements.add(el));
      } catch (ex) {
        if (!options?.forgiving) throw ex;
      }
    }

    return Array.from(elements);
  }
}
