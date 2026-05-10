import { Child, Descendant, NextSibling, SubsequentSibling } from '../combinators';
import { extPseudoClasses } from '../extendedPseudoClasses';
import { Selector } from '../types';

import { RawMatches, RawQuery } from './raw';
import { CombToken, IRToken } from './tokenize';

/**
 * Builds a final, optimized query out of intermediate representation tokens.
 */
export function plan(tokens: IRToken[]): Selector {
  if (tokens.length === 0) return [];

  const steps: Selector = [];
  let haveContextualStep = false; // true after we've emitted anything that can serve as context for requiresContext

  const emitBridge = (comb: CombToken) => {
    switch (comb.literal) {
      case ' ':
        steps.push(new Descendant());
        break;
      case '+':
        steps.push(new NextSibling());
        break;
      case '~':
        steps.push(new SubsequentSibling());
        break;
      case '>':
        steps.push(new Child());
        break;
      default:
        throw new Error(`Unknown combinator: "${comb.literal}"`);
    }
    haveContextualStep = true;
  };

  const emitExt = (name: keyof typeof extPseudoClasses, args: string) => {
    const extClass = extPseudoClasses[name];
    if (!extClass) {
      throw new Error(`Unknown extended pseudo-class ":${name}"`);
    }
    if (extClass.requiresContext && !haveContextualStep) {
      steps.push(new RawQuery('*'));
      haveContextualStep = true;
    }
    steps.push(new extClass(args));
    haveContextualStep = true;
  };

  // ---------- Relative selector (starts with a combinator) ----------
  const startsWithCombinator = tokens[0].kind === 'comb';
  if (startsWithCombinator) {
    /**
     * Special case: if the selector start with a combinator, then it's a relative selector.
     * In this case we cannot optimize by merging adjacent "raw" runs; instead, we must rely on filtering via
     * .matches at each step.
     * This behavior is required to support the :has() pseudo-class.
     * See: https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_selectors/Selector_structure#relative_selector
     */
    for (let i = 0; i < tokens.length; i++) {
      const t = tokens[i];

      switch (t.kind) {
        case 'comb': {
          // Mirror default path invariants
          const next = tokens[i + 1];
          if (!next) {
            throw new Error('Relative selector ends with a dangling combinator');
          }
          if (next.kind === 'comb') {
            throw new Error('Multiple subsequent combinator tokens in relative selector');
          }
          emitBridge(t);
          break;
        }
        case 'raw':
          // In relative mode we avoid merging raw chunks and rely on stepwise matching semantics.
          steps.push(new RawMatches(t.literal));
          haveContextualStep = true;
          break;
        case 'ext':
          emitExt(t.name, t.args);
          break;
      }
    }
    return steps;
  }

  // ---------- Default path (non-relative selector) ----------
  // Accumulates CSS for coalesced raw runs; trims on flush.
  let cssBuilder = '';
  const flushRaw = () => {
    const raw = cssBuilder.trim();
    if (raw) {
      steps.push(new RawQuery(raw));
      haveContextualStep = true;
    }
    cssBuilder = '';
  };

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];

    switch (t.kind) {
      case 'raw': {
        const prevStep = steps[steps.length - 1];
        const isPrevComb =
          prevStep instanceof Child ||
          prevStep instanceof Descendant ||
          prevStep instanceof NextSibling ||
          prevStep instanceof SubsequentSibling;

        if (isPrevComb) {
          steps.push(new RawMatches(t.literal));
        } else {
          cssBuilder += t.literal;
        }
        break;
      }

      case 'comb': {
        const nextTok = tokens[i + 1];
        if (!nextTok) {
          throw new Error('Last token is a dangling combinator');
        }
        if (nextTok.kind === 'comb') {
          throw new Error('Multiple subsequent combinator tokens');
        }

        if (cssBuilder.length > 0 && nextTok.kind === 'raw') {
          // Prefer declarative bridging for performance.
          cssBuilder += t.literal;
        } else if (nextTok.kind === 'raw') {
          switch (t.literal) {
            case ' ':
              // No bridging necessary. The next step is going to be RawQuery, which matches descendant elements by nature.
              break;
            case '>':
              // Micro-optimization: Use :scope to match direct descendants when calling querySelectorAll.
              // Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/:scope
              cssBuilder += ':scope' + t.literal;
              break;
            default:
              // :scope-d queries do not match siblings, so bridge imperatively.
              emitBridge(t);
          }
        } else {
          // Next is ext; end the merged raw run and bridge imperatively.
          flushRaw();
          emitBridge(t);
        }
        break;
      }

      case 'ext':
        flushRaw();
        emitExt(t.name, t.args);
        break;
    }
  }

  flushRaw();
  return steps;
}
