import * as CSSTree from 'css-tree';

import { extractStyleDeclarations, parseDeclarations } from './declarations';
import { parseASTSelectorList } from './selectorList';
import { Declaration, Rule } from './types';

export function parse(rule: string): Rule {
  const ast = CSSTree.parse(rule, { context: 'selectorList', positions: true }) as CSSTree.SelectorList;

  let declarations: Declaration[] | undefined;

  if (ast.children.size === 1) {
    // :style() pseudo-class only makes sense if there's a single selector in the selector list.
    const decl = extractStyleDeclarations(ast.children.first!);
    if (decl !== null) {
      declarations = parseDeclarations(decl);
    }
  }

  const selectorList = parseASTSelectorList(ast, rule);

  return declarations ? { type: 'style', declarations, selectorList } : { type: 'hide', selectorList };
}
