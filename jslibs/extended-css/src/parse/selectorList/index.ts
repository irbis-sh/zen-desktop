import * as CSSTree from 'css-tree';

import { SelectorList } from '../types';

import { plan } from './plan';
import { tokenize } from './tokenize';

export function parseRawSelectorList(raw: string): SelectorList {
  const ast = CSSTree.parse(raw, { context: 'selectorList', positions: true }) as CSSTree.SelectorList;
  return parseASTSelectorList(ast, raw);
}

export function parseASTSelectorList(ast: CSSTree.SelectorList, raw: string): SelectorList {
  const tokens = tokenize(ast, raw);
  return tokens.map(plan);
}
