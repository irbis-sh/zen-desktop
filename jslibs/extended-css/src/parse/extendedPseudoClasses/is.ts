import { SelectorExecutor } from '../../engine/selectorExecutor';
import { parseRawSelectorList } from '../selectorList';
import { Step } from '../types';

export class Is implements Step {
  static requiresContext = true;

  private executor: SelectorExecutor;

  constructor(selector: string) {
    this.executor = new SelectorExecutor(parseRawSelectorList(selector));
  }

  run(input: Element[]): Element[] {
    const matched: Set<Element> = new Set();

    const matchedEls = this.executor.match(document.documentElement, { forgiving: true });
    for (const element of matchedEls) {
      matched.add(element);
    }

    return input.filter((el) => matched.has(el));
  }

  toString() {
    // A complete implementation would store the selector passed to the
    // constructor. However, since it's unused in production methods, we avoid
    // the memory overhead at the cost of slightly less thorough testing.
    return ':Is(...)';
  }
}
