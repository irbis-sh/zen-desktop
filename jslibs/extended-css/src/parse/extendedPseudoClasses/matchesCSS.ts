import { Step } from '../types';
import { parseRegexpLiteral, parseWildcardPattern } from '../utils/parseRegexp';

export class MatchesCSS implements Step {
  static requiresContext = true;

  private pseudoElement?: string;
  private property: string;
  private valueRe?: RegExp;
  private valueSearch?: string;

  constructor(args: string) {
    const { value, pseudoElement, property } = this.parseArgs(args);
    this.pseudoElement = pseudoElement;
    this.property = property;

    const re = parseRegexpLiteral(value);
    if (re !== null) {
      this.valueRe = re;
      return;
    }
    if (value.includes('*')) {
      const re = parseWildcardPattern(value);
      if (re !== null) {
        this.valueRe = re;
        return;
      }
    }
    this.valueSearch = value;
  }

  private parseArgs(args: string): { pseudoElement?: string; property: string; value: string } {
    const parts = args.split(',').map((s) => s.trim());

    let pseudoElement: string | undefined;
    let propertyValue: string;

    if (parts.length === 2) {
      pseudoElement = parts[0];
      propertyValue = parts[1];
    } else {
      propertyValue = parts[0];
    }

    const colonIndex = propertyValue.indexOf(':');
    if (colonIndex === -1) {
      throw new Error('Invalid matches-css syntax: missing colon separator');
    }

    const property = propertyValue.substring(0, colonIndex).trim();
    const value = propertyValue.substring(colonIndex + 1).trim();

    if (!property || !value) {
      throw new Error('Invalid matches-css syntax: empty property or value');
    }

    return { pseudoElement, property, value };
  }

  run(input: Element[]): Element[] {
    return input.filter((element) => this.matchesElement(element));
  }

  private matchesElement(element: Element): boolean {
    try {
      const computedStyle = window.getComputedStyle(element, this.pseudoElement);
      const actualValue = computedStyle.getPropertyValue(this.property);

      if (this.valueRe) {
        return this.valueRe.test(actualValue);
      } else if (this.valueSearch) {
        return actualValue === this.valueSearch;
      }

      return false;
    } catch {
      return false;
    }
  }

  toString() {
    let body = '';
    if (this.pseudoElement) {
      body = this.pseudoElement + ', ';
    }
    body += this.property + ': ';
    if (this.valueRe) {
      body += this.valueRe.toString();
    } else if (this.valueSearch) {
      body += this.valueSearch.toString();
    }

    return `:MatchesCSS(${body})`;
  }
}
