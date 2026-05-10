import { removeFromStorage, validateUntrustedStorageValue } from './helpers/storage';

export function setLocalStorageItem(key: string, value: string) {
  if (typeof key !== 'string') {
    throw new Error(`key should be string, is ${key}`);
  } else if (typeof value !== 'string') {
    throw new Error(`value should be string, is ${value}`);
  }

  const validatedValue = validateUntrustedStorageValue(value);
  if (validatedValue === '$remove$') {
    removeFromStorage(localStorage, key);
  } else {
    localStorage.setItem(key, validatedValue);
  }
}
