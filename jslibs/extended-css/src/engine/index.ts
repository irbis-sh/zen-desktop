import { parse } from '../parse';
import { Declaration } from '../parse/types';
import { createLogger } from '../utils/logger';
import { throttle } from '../utils/throttle';

import { SelectorExecutor } from './selectorExecutor';

const logger = createLogger('engine');

export class Engine {
  private readonly rules: Array<{ executor: SelectorExecutor; className: string }>;
  private readonly target = document.documentElement;

  private readonly appliedClasses = new Map<Element, Set<string>>();
  private observer: MutationObserver | null = null;
  private styleEl: HTMLStyleElement | null = null;

  constructor(rules: string) {
    logger.debug('Initializing engine');
    const parsed = this.parseRules(rules);
    this.rules = parsed.rules;
    this.createStyleSheet(parsed.cssText);
  }

  start(): void {
    logger.debug(`Starting with ${this.rules.length} rules`);

    this.applyQueries();

    if (document.readyState !== 'complete') {
      document.addEventListener(
        'DOMContentLoaded',
        () => {
          this.applyQueries();
        },
        { once: true },
      );
    }

    this.registerObserver();
  }

  /**
   * Tears down the mutation observer and restores styles of all affected elements to their original state.
   * Engine is not usable after this method is called.
   */
  stop(): void {
    if (this.observer) {
      this.observer.disconnect();
      this.observer = null;
    }

    for (const el of this.appliedClasses.keys()) {
      this.restoreAllClasses(el);
    }

    this.appliedClasses.clear();

    if (this.styleEl) {
      this.styleEl.remove();
      this.styleEl = null;
    }

    logger.debug('Engine stopped');
  }

  private parseRules(rules: string): {
    rules: Array<{ executor: SelectorExecutor; className: string }>;
    cssText: string;
  } {
    const lines = rules.split('\n');

    const parsedRules: Array<{ executor: SelectorExecutor; className: string }> = [];
    let cssRules = '';
    let ruleIndex = 0;
    const hideClassName = 'zenc_hide';
    let hideClassAdded = false;

    const addStyleRule = (executor: SelectorExecutor, declarations: Declaration[]) => {
      if (declarations.length === 0) return;

      const className = `zenc_${ruleIndex++}`;
      cssRules += this.buildCssRule(className, declarations);
      parsedRules.push({ executor, className });
    };

    const addHideRule = (executor: SelectorExecutor) => {
      if (!hideClassAdded) {
        cssRules += this.buildCssRule(hideClassName, [{ property: 'display', value: 'none', important: true }]);
        hideClassAdded = true;
      }
      parsedRules.push({ executor, className: hideClassName });
    };

    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed.length === 0) continue;

      try {
        const rule = parse(trimmed);
        const executor = new SelectorExecutor(rule.selectorList);

        switch (rule.type) {
          case 'hide':
            addHideRule(executor);
            break;
          case 'style':
            addStyleRule(executor, rule.declarations);
            break;
          default:
            throw new Error(`Unknown rule type ${(rule as any).type}`);
        }
      } catch (ex) {
        logger.error(`Failed to parse rule: "${line}"`, ex);
      }
    }

    return { rules: parsedRules, cssText: cssRules };
  }

  private buildCssRule(className: string, declarations: Declaration[]): string {
    const declText = declarations
      .map((decl) => `${decl.property}:${decl.value}${decl.important ? '!important' : ''}`)
      .join(';');

    return `.${className}{${declText}}`;
  }

  private applyQueries(): void {
    const start = performance.now();

    const desiredClasses = this.collectClassMatches();
    const { added, removed } = this.applyClasses(desiredClasses);

    const elapsed = (performance.now() - start).toFixed(2);
    logger.debug(`Applied ${added} classes, removed ${removed} classes ` + `in ${elapsed}ms`);
  }

  private collectClassMatches(): Map<Element, Set<string>> {
    const matches = new Map<Element, Set<string>>();

    for (const rule of this.rules) {
      try {
        const els = rule.executor.match(this.target);
        for (const el of els) {
          if (!(el instanceof HTMLElement)) continue;
          let classes = matches.get(el);
          if (!classes) {
            classes = new Set<string>();
            matches.set(el, classes);
          }

          classes.add(rule.className);
        }
      } catch (ex) {
        logger.error(`Failed to apply rule`, ex);
      }
    }

    return matches;
  }

  private applyClasses(desired: Map<Element, Set<string>>): {
    added: number;
    removed: number;
  } {
    let added = 0;
    let removed = 0;

    for (const el of this.appliedClasses.keys()) {
      const applied = this.appliedClasses.get(el)!;
      const desiredClasses = desired.get(el);
      if (!desiredClasses) {
        // No desired classes for this element (no selector matched), remove all applied.
        for (const className of applied.values()) {
          el.classList.remove(className);
          removed++;
        }
        this.appliedClasses.delete(el);
        continue;
      }

      for (const className of applied.values()) {
        if (!desiredClasses.has(className)) {
          el.classList.remove(className);
          applied.delete(className);
          removed++;
        }
      }
    }

    for (const [el, desiredClasses] of desired) {
      if (!(el instanceof HTMLElement)) continue;

      let applied = this.appliedClasses.get(el);
      if (!applied) {
        applied = new Set();
        this.appliedClasses.set(el, applied);
      }

      for (const className of desiredClasses) {
        if (!applied.has(className)) {
          el.classList.add(className);
          applied.add(className);
          added++;
        }
      }
    }

    return { added, removed };
  }

  private restoreAllClasses(el: Element): void {
    if (!(el instanceof HTMLElement)) return;
    const applied = this.appliedClasses.get(el);
    if (!applied) return;

    for (const className of applied.values()) {
      el.classList.remove(className);
    }

    this.appliedClasses.delete(el);
  }

  private createStyleSheet(cssText: string): void {
    if (!this.styleEl) {
      this.styleEl = document.createElement('style');
      (document.head || document.documentElement).appendChild(this.styleEl);
    }

    this.styleEl.textContent = cssText;
  }

  private registerObserver(): void {
    const options: MutationObserverInit = {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['id', 'class'],
    };

    const cb = throttle((observer: MutationObserver) => {
      observer.disconnect();
      this.applyQueries();
      observer.observe(this.target, options);
    }, 100);

    this.observer = new MutationObserver((mutations, observer) => {
      if (mutations.length === 0) return;

      cb(observer);
    });

    this.observer.observe(this.target, options);
  }
}
