import { Step } from '../types';
import { parseRegexpLiteral } from '../utils/parseRegexp';

export class Contains implements Step {
  static requiresContext = true;

  private textRe?: RegExp;
  private textSearch?: string;

  constructor(text: string) {
    const re = parseRegexpLiteral(text);
    if (re !== null) {
      this.textRe = re;
      return;
    }
    this.textSearch = text;
  }

  run(input: Element[]) {
    if (this.textRe) {
      return input.filter((e) => e.textContent !== null && this.textRe!.test(e.textContent));
    } else if (this.textSearch) {
      return input.filter((e) => e.textContent !== null && e.textContent.includes(this.textSearch!));
    }
    return [];
  }

  toString() {
    return `:Contains(${(this.textRe || this.textSearch)!.toString()})`;
  }
}
