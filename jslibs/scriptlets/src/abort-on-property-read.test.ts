import { expect, test, describe } from '@jest/globals';

import { abortOnPropertyRead } from './abort-on-property-read';

describe('abort-on-property-read', () => {
  afterEach(() => {
    delete (window as any).PROPERTY;
    delete (window as any).test;
    delete (window as any).prop1;
  });

  test('abort on single prop read', () => {
    abortOnPropertyRead('test');

    expect(() => {
      (window as any).test;
    }).toThrow(ReferenceError);
  });

  test('dont abort on prop write', () => {
    abortOnPropertyRead('test');

    expect(() => {
      (window as any).test = 123;
    }).not.toThrow(ReferenceError);
  });

  test('abort on long chain read', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnPropertyRead('test.prop.prop2');

    expect(() => {
      (window as any).test.prop.prop2;
    }).toThrow(ReferenceError);
  });

  test('dont abort on long chain write', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnPropertyRead('test.prop.prop2');

    expect(() => {
      (window as any).test.prop.prop2 = 123;
    }).not.toThrow(ReferenceError);
  });

  test('document.querySelectorAll', () => {
    abortOnPropertyRead('document.querySelectorAll');

    expect(() => {
      window.document.querySelectorAll('test');
    }).toThrow(ReferenceError);
  });

  test('properties inside chain are not initialized by scriptlet', () => {
    abortOnPropertyRead('prop1.prop2.prop3');

    expect((window as any).prop1).toBeUndefined();

    (window as any).prop1 = {};

    expect((window as any).prop1.prop2).toBeUndefined();

    (window as any).prop1.prop2 = {};

    expect(() => {
      (window as any).prop1.prop2.prop3;
    }).toThrow(ReferenceError);
  });
});
