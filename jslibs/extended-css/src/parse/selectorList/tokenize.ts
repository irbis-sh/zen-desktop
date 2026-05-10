import * as CSSTree from 'css-tree';

import { extPseudoClasses } from '../extendedPseudoClasses';
import { getLiteral } from '../utils/getLiteral';

/**
 * Intermediate representation token.
 */
export type IRToken = RawToken | CombToken | ExtToken;

/**
 * Token representation of a selector.
 */
export type SelectorTokens = IRToken[];

/**
 * Parses a selector/selector list into an intermediate token representation.
 */
export function tokenize(ast: CSSTree.SelectorList, raw: string): SelectorTokens[] {
  const result: IRToken[][] = [];

  ast.children.forEach((selectorNode) => {
    if (selectorNode.type === 'Selector') {
      result.push(parseTokens(selectorNode, raw));
    }
  });

  return result;
}

function parseTokens(ast: CSSTree.CssNode, selector: string): IRToken[] {
  const out: IRToken[] = [];
  let cssBuf = '';

  const flushRaw = () => {
    const t = cssBuf.trim();
    if (t.length > 0) {
      out.push(new RawToken(t));
    }
    cssBuf = '';
  };

  const getNodeLit = (node: CSSTree.CssNode) => getLiteral(node, selector);

  CSSTree.walk(ast, (node) => {
    switch (node.type) {
      case 'Selector':
        return;

      case 'IdSelector':
      case 'ClassSelector':
      case 'TypeSelector':
      case 'AttributeSelector':
        cssBuf += getNodeLit(node);
        if (node.type === 'AttributeSelector') return CSSTree.walk.skip;
        return;

      case 'Combinator':
        flushRaw();
        out.push(new CombToken(node.name));
        return;

      case 'PseudoClassSelector': {
        const name = node.name.toLowerCase();
        if (name in extPseudoClasses) {
          // Optimization: if :has/:is/:not contains only standard CSS args,
          // treat the whole thing as raw CSS for native evaluation.
          if (getNativeSupport(name) && hasOnlyStandardArgs(node)) {
            cssBuf += getNodeLit(node);
            return CSSTree.walk.skip;
          }

          flushRaw();

          const arg = node.children?.first;
          if (arg == undefined) {
            throw new Error(`:${name}: expected an argument, got null/undefined`);
          }

          const argValue = getNodeLit(arg);

          out.push(new ExtToken(name as keyof typeof extPseudoClasses, argValue));
        } else {
          cssBuf += getNodeLit(node);
        }
        return CSSTree.walk.skip;
      }

      default:
        throw new Error(`Unexpected node type: ${node.type}`);
    }
  });

  flushRaw();

  return out;
}

/**
 * Feature-detection for pseudo-classes that can be evaluated natively
 * when their arguments contain only standard CSS.
 */
function getNativeSupport(pseudoSelector: string): boolean {
  switch (pseudoSelector) {
    case 'has':
      return typeof CSS !== 'undefined' && !!CSS.supports?.('selector(:has(*))');
    case 'is':
      return typeof CSS !== 'undefined' && !!CSS.supports?.('selector(:is(*))');
    case 'not':
      return typeof CSS !== 'undefined' && !!CSS.supports?.('selector(:not(*))');
    default:
      return false;
  }
}

/**
 * Checks whether a pseudo-class node's argument subtree contains only
 * standard CSS (no extended pseudo-classes that require JS evaluation).
 */
function hasOnlyStandardArgs(node: CSSTree.PseudoClassSelector): boolean {
  let result = true;
  CSSTree.walk(node, {
    visit: 'PseudoClassSelector',
    enter(child) {
      if (child === node) return;
      const name = child.name.toLowerCase();
      if (name in extPseudoClasses) {
        if (getNativeSupport(name)) {
          if (!hasOnlyStandardArgs(child)) {
            result = false;
            return this.break;
          }
        } else {
          result = false;
          return this.break;
        }
      }
    },
  });
  return result;
}

/**
 * Raw query token.
 */
export class RawToken {
  public kind: 'raw' = 'raw';
  constructor(public literal: string) {}
  toString() {
    return `RawTok(${this.literal})`;
  }
}

/**
 * Combinator token.
 */
export class CombToken {
  public kind: 'comb' = 'comb';
  constructor(public literal: string) {}
  toString() {
    return `CombTok(${this.literal})`;
  }
}

/**
 * Extended pseudo class token.
 */
export class ExtToken {
  public kind: 'ext' = 'ext';
  constructor(
    public name: keyof typeof extPseudoClasses,
    public args: string,
  ) {}

  toString() {
    return `ExtTok(:${this.name}(${this.args}))`;
  }
}
