import { Step } from '../types';

export class RawQuery implements Step {
  constructor(private query: string) {}

  run(input: Element[]) {
    const res = [];
    for (const el of input) {
      const selected = el.querySelectorAll(this.query);
      for (const el of selected) {
        res.push(el);
      }
    }
    return res;
  }

  toString() {
    return `RawQuery(${this.query})`;
  }
}

export class RawMatches implements Step {
  constructor(private query: string) {}

  run(input: Element[]) {
    if (this.query === '*') {
      return input;
    }
    return input.filter((el) => el.matches(this.query));
  }

  toString() {
    return `RawMatches(${this.query})`;
  }
}
