import { matchStack } from './matchStack';
import { parseRegexpFromString, parseRegexpLiteral } from './parseRegexp';

type PropPath = string[];

/**
 * createPrune creates a JSON/object pruner for use in json-prune and related scriptlets.
 * @param propsToRemove - Space-separated list of property paths to remove from the object.
 * @param requiredProps - Space-separated list of property paths that must be present in the object for pruning to occur.
 * @param stack - Regular expression/substring to match against the stack trace.
 * @returns Function that prunes the provided object in place.
 */
export function createPrune(propsToRemove: string, requiredProps?: string, stack?: string) {
  const parsedPropsToRemove = parsePropPaths(propsToRemove);
  const parsedRequiredProps = parsePropPaths(requiredProps);
  let stackRe: RegExp | null = null;
  if (typeof stack === 'string') {
    stackRe = parseRegexpLiteral(stack) || parseRegexpFromString(stack);
  }

  return function prune(obj: any): void {
    if (typeof obj !== 'object') {
      return;
    }

    if (stackRe !== null && !matchStack(stackRe)) {
      return;
    }

    if (parsedRequiredProps.length > 0) {
      let matched = false;
      for (const propToMatch of parsedRequiredProps) {
        if (matchesPath(obj, propToMatch)) {
          matched = true;
          break;
        }
      }
      if (!matched) {
        return;
      }
    }

    for (const propToRemove of parsedPropsToRemove) {
      prunePath(obj, propToRemove);
    }
  };
}

function prunePath(obj: any, path: PropPath) {
  if (path.length === 0 || obj == null) {
    return;
  }

  const [segment, ...rest] = path;

  if (segment === '*') {
    const objProps = Object.getOwnPropertyNames(obj).filter(
      (prop) => typeof obj[prop] === 'object' || Array.isArray(obj[prop]),
    );
    for (const prop of objProps) {
      prunePath(obj[prop], path);
      prunePath(obj[prop], rest);
    }
    return;
  }
  if (segment === '[]') {
    if (!Array.isArray(obj)) {
      return;
    }
    for (let i = 0; i < obj.length; i++) {
      prunePath(obj[i], rest);
    }
    return;
  }

  if (Object.hasOwn(obj, segment)) {
    if (rest.length === 0) {
      delete obj[segment];
    } else {
      prunePath(obj[segment], rest);
    }
  }
}

function matchesPath(obj: any, path: PropPath): boolean {
  if (obj == null) {
    return false;
  }
  if (path.length === 0) {
    return true;
  }

  if (path[0] === '*') {
    const objProps = Object.getOwnPropertyNames(obj).filter(
      (prop) => typeof obj[prop] === 'object' || Array.isArray(obj[prop]),
    );
    for (const prop of objProps) {
      if (matchesPath(obj[prop], path) || matchesPath(obj[prop], path.slice(1))) {
        return true;
      }
    }
    return false;
  }
  if (path[0] === '[]') {
    if (!Array.isArray(obj)) {
      return false;
    }
    for (let i = 0; i < obj.length; i++) {
      if (matchesPath(obj[i], path.slice(1))) {
        return true;
      }
    }
    return false;
  }

  if (Object.hasOwn(obj, path[0])) {
    return matchesPath(obj[path[0]], path.slice(1));
  }
  return false;
}

function parsePropPaths(propPaths?: string): PropPath[] {
  if (typeof propPaths !== 'string') {
    return [];
  }

  return propPaths
    .split(/\s+/)
    .filter(Boolean)
    .map((prop) => prop.split('.'));
}
