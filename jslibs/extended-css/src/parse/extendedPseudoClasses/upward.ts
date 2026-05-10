import { Step } from '../types';

export class Upward implements Step {
  static requiresContext = true;

  private distance?: number;
  private selector?: string;

  constructor(subject: string) {
    const distance = Number.parseInt(subject);
    if (!Number.isNaN(distance)) {
      if (distance < 1 || distance >= 256) {
        throw new Error('Invalid distance value');
      }
      this.distance = distance;
    } else {
      this.selector = subject;
    }
  }

  run(input: Element[]): Element[] {
    if (this.distance) {
      return this.matchDistance(input);
    } else if (this.selector) {
      return this.matchSelector(input);
    }
    return [];
  }

  private matchDistance(input: Element[]): Element[] {
    const res = [];
    for (const el of input) {
      let ancestor: Element | null = el;
      for (let i = 0; i < this.distance!; i++) {
        ancestor = ancestor.parentElement;
        if (ancestor === null) {
          break;
        }
      }
      if (ancestor !== null) {
        res.push(ancestor);
      }
    }
    return res;
  }

  private matchSelector(input: Element[]): Element[] {
    const res = [];
    for (const el of input) {
      let ancestor = el.parentElement;
      while (ancestor !== null) {
        if (ancestor.matches(this.selector!)) {
          res.push(ancestor);
          break;
        }
        ancestor = ancestor.parentElement;
      }
    }
    return res;
  }

  toString() {
    return `:Upward(${(this.distance || this.selector)!.toString()})`;
  }
}
