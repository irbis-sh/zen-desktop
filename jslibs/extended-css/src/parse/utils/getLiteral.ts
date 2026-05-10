import { CssNode } from 'css-tree';

/**
 * Gets the literal text represented by the given AST node.
 * Assumes the node was parsed with { positions: true } option.
 */
export function getLiteral(node: CssNode, raw: string): string {
  return raw.slice(node.loc!.start.offset, node.loc!.end.offset);
}
