import { Step } from '../types';
import { parseRegexpLiteral, parseWildcardPattern } from '../utils/parseRegexp';

export class MatchesAttr implements Step {
  static requiresContext = true;

  private name: RegExp | string;
  private value?: RegExp | string;

  constructor(arg: string) {
    if (arg.length === 0) {
      throw new Error('Invalid syntax');
    }

    const parsed = this.parseArg(arg);
    this.name = parsed.name;
    this.value = parsed.value;
  }

  run(input: Element[]): Element[] {
    return input.filter((element) => this.matchesElement(element));
  }

  toString() {
    let body = '';
    body += this.name.toString();
    if (this.value) {
      body += '=' + this.value.toString();
    }

    return `:MatchesAttr(${body})`;
  }

  private parseArg(arg: string): { name: RegExp | string; value?: RegExp | string } {
    const parts = arg.split('=');
    if (parts.length === 1) {
      return {
        name: this.parseMatcher(parts[0]),
      };
    } else {
      return {
        name: this.parseMatcher(parts[0]),
        value: this.parseMatcher(parts[1]),
      };
    }
  }

  private parseMatcher(matcher: string): RegExp | string {
    let trimmed = matcher;
    if (trimmed.length > 2) {
      // Remove optional quotes.
      const first = trimmed[0];
      const last = trimmed[trimmed.length - 1];
      if (first === last && (first === '"' || first === "'")) {
        trimmed = trimmed.slice(1, -1);
      }
    }

    const re = parseRegexpLiteral(trimmed);
    if (re !== null) {
      return re;
    }
    if (trimmed.includes('*')) {
      const re = parseWildcardPattern(trimmed);
      if (re !== null) {
        return re;
      }
    }
    return trimmed;
  }

  private matchesElement(element: Element): boolean {
    for (const attr of element.attributes) {
      if (!this.matches(this.name, attr.name)) {
        continue;
      }
      if (this.value !== undefined && !this.matches(this.value, attr.value)) {
        continue;
      }
      return true;
    }
    return false;
  }

  private matches(test: RegExp | string, value: string) {
    if (typeof test === 'string') {
      return test === value;
    } else {
      return test.test(value);
    }
  }
}
