import { SelectorExecutor } from '../../engine/selectorExecutor';
import { parseRawSelectorList } from '../selectorList';
import { Step } from '../types';

export class Not implements Step {
  static requiresContext = true;

  private executor: SelectorExecutor;

  constructor(selector: string) {
    this.executor = new SelectorExecutor(parseRawSelectorList(selector));
  }

  run(input: Element[]): Element[] {
    const matched: Set<Element> = new Set();

    const matchedEls = this.executor.match(document.documentElement);
    for (const element of matchedEls) {
      matched.add(element);
    }

    const filtered = [];
    for (const element of input) {
      if (!matched.has(element)) {
        filtered.push(element);
      }
    }
    return filtered;
  }

  toString() {
    // A complete implementation would store the selector passed to the
    // constructor. However, since it's unused in production methods, we avoid
    // the memory overhead at the cost of slightly less thorough testing.
    return ':Not(...)';
  }
}
