import { parseRegexpLiteral } from './parseRegexp';

const literalValues = new Set([
  'undefined',
  'false',
  'true',
  'null',
  'yes',
  'no',
  'on',
  'off',
  'accept',
  'accepted',
  'reject',
  'rejected',
  'allowed',
  'denied',
  'forbidden',
  'forever',
  '',
]);

/**
 * Checks whether the value can be used in untrusted set-local-storage-item and set-session-storage-item scriptlets
 * and potentially converts it to a canonical form.
 * @param value The value to validate.
 * @returns The canonical value if it is valid.
 * @throws An error if the value is invalid.
 */
export function validateUntrustedStorageValue(value: string): string {
  if (literalValues.has(value.toLowerCase())) {
    return value;
  } else if (value === 'emptyArr') {
    return '[]';
  } else if (value === 'emptyObj') {
    return '{}';
  } else if (value === '$remove$') {
    return '$remove$';
  }

  const int = parseInt(value);
  if (!isNaN(int) && int >= 0 && int <= 32767) {
    return value;
  }

  throw new Error('Invalid value');
}

export function removeFromStorage(storage: Storage, key: string): void {
  const regexp = parseRegexpLiteral(key);
  if (regexp !== null) {
    const storageKeys = Object.keys(storage);
    for (const storageKey of storageKeys) {
      if (regexp.test(storageKey)) {
        storage.removeItem(storageKey);
      }
    }
  } else {
    storage.removeItem(key);
  }
}
