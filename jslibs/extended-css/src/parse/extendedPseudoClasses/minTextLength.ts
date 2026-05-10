import { Step } from '../types';

export class MinTextLength implements Step {
  static requiresContext = true;

  private n: number;

  constructor(arg: string) {
    const n = Number.parseInt(arg, 10);
    if (Number.isNaN(n)) {
      throw new Error('Invalid text length');
    }
    if (n < 1) {
      throw new Error('Text length cannot be less than 1');
    }
    this.n = n;
  }

  run(input: Element[]): Element[] {
    return input.filter((el) => el.textContent !== null && el.textContent.length >= this.n);
  }

  toString() {
    return `:MinTextLength(${this.n})`;
  }
}
